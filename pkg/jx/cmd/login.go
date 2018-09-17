package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/chromedp/runner"
	"github.com/hpcloud/tail"
	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/jenkins-x/jx/pkg/kube"
	jxlog "github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const (
	defaultNamespace       = "jx"
	UserOnboardingEndpoint = "/api/v1/users"
	SsoCookieName          = "sso-cdx"
)

// Login holds the login information
type Login struct {
	Data UserLoginInfo `form:"data,omitempty" json:"data,omitempty" yaml:"data,omitempty" xml:"data,omitempty"`
}

// UserLoginInfo user login information
type UserLoginInfo struct {
	// The kubernetes api server public CA data
	Ca string `form:"ca,omitempty" json:"ca,omitempty" yaml:"ca,omitempty" xml:"ca,omitempty"`
	// The login username of the user
	Login string `form:"login,omitempty" json:"login,omitempty" yaml:"login,omitempty" xml:"login,omitempty"`
	// The kubernetes api server address
	Server string `form:"server,omitempty" json:"server,omitempty" yaml:"server,omitempty" xml:"server,omitempty"`
	// The login token of the user
	Token string `form:"token,omitempty" json:"token,omitempty" yaml:"token,omitempty" xml:"token,omitempty"`
}

// LoginOptions options for login command
type LoginOptions struct {
	CommonOptions

	URL string
}

var (
	login_long = templates.LongDesc(`
		Onboards an user into the CloudBees application and configures the Kubernetes client configuration.

		A CloudBess app can be created as an addon with 'jx create addon cloudbees'`)

	login_example = templates.Examples(`
		# Onboard into CloudBees application
		jx login`)
)

func NewCmdLogin(f Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &LoginOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			Out:     out,
			Err:     errOut,
		},
	}
	cmd := &cobra.Command{
		Use:     "login",
		Short:   "Onboard an user into the CloudBees application",
		Long:    login_long,
		Example: login_example,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}

	cmd.Flags().StringVarP(&options.URL, "url", "u", "", "The URL of the CloudBees application")

	return cmd
}

func (o *LoginOptions) Run() error {
	userLoginInfo, err := o.Login()
	if err != nil {
		return errors.Wrap(err, "loging into the CloudBees application")
	}

	err = kube.UpdateConfig(defaultNamespace, userLoginInfo.Server, userLoginInfo.Ca, userLoginInfo.Login, userLoginInfo.Token)
	if err != nil {
		return errors.Wrap(err, "updating the ~/kube/config file")
	}

	jxlog.Infof("You are %s. You credentials are stored in %s file.\n",
		util.ColorInfo("successfully logged in"), util.ColorInfo("~/.kube/config"))

	teamOptions := TeamOptions{}
	err = teamOptions.Run()
	if err != nil {
		return errors.Wrap(err, "switching team")
	}

	return nil
}

func (o *LoginOptions) Login() (*UserLoginInfo, error) {
	url := o.URL
	if url == "" {
		return nil, errors.New("please povide the URL of the CloudBees application in '--url' option")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	userDataDir, err := ioutil.TempDir("/tmp", "jx-login-chrome-userdata-dir")
	if err != nil {
		return nil, errors.Wrap(err, "creating the chrome user data dir")
	}
	defer os.RemoveAll(userDataDir)

	netLogFile := filepath.Join(userDataDir, "net-logs.json")
	options := func(m map[string]interface{}) error {
		m["start-url"] = o.URL
		m["user-data-dir"] = userDataDir
		m["log-net-log"] = netLogFile
		m["net-log-capture-mode"] = "IncludeCookiesAndCredentials"
		m["v"] = 1
		return nil
	}

	r, err := runner.New(options)
	if err != nil {
		return nil, errors.Wrap(err, "creating chrome runner")
	}

	err = r.Start(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "starting chrome")
	}

	t, err := tail.TailFile(netLogFile, tail.Config{
		Follow: true,
		Logger: log.New(ioutil.Discard, "", log.LstdFlags)})
	if err != nil {
		return nil, errors.Wrap(err, "reading the netlog file")
	}
	cookie := ""
	pattern := fmt.Sprintf("%s=", SsoCookieName)
	for line := range t.Lines {
		if strings.Contains(line.Text, pattern) {
			cookie = ExtractSsoCookie(line.Text)
			break
		}
	}

	if cookie == "" {
		return nil, errors.New("failed to log into the CloudBees application")
	}

	userLoginInfo, err := o.OnboardUser(cookie)
	if err != nil {
		return nil, errors.Wrap(err, "onboarding user")
	}

	err = r.Shutdown(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "shutting down Chrome")
	}

	err = r.Wait()
	if err != nil {
		return nil, errors.Wrap(err, "waiting for Chrome  to exit")
	}

	return userLoginInfo, nil
}

func (o *LoginOptions) OnboardUser(cookie string) (*UserLoginInfo, error) {
	client := http.Client{}
	req, err := http.NewRequest("POST", o.onboardingURL(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "building onboarding request")
	}
	req.Header.Add("Accept", "application/json")
	if cookie == "" {
		return nil, errors.New("empty SSO cookie")
	}
	ssoCookie := http.Cookie{Name: SsoCookieName, Value: cookie}
	req.AddCookie(&ssoCookie)
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "executing onboarding request")
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading user onboarding information from response body")
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("user onboarding status code: %d, error: %s", resp.StatusCode, string(body))
	}

	login := &Login{}
	if err := json.Unmarshal(body, login); err != nil {
		return nil, errors.Wrap(err, "parsing the login information from response")
	}

	return &login.Data, nil
}

func (o *LoginOptions) onboardingURL() string {
	url := o.URL
	if strings.HasPrefix(url, "/") {
		url = strings.TrimPrefix(url, "/")
	}
	return url + UserOnboardingEndpoint
}

func ExtractSsoCookie(text string) string {
	cookiePattern := fmt.Sprintf("%s=", SsoCookieName)
	start := strings.Index(text, cookiePattern)
	if start < 0 {
		return ""
	}
	end := -1
	cookieStart := start + len(cookiePattern)
	for i, ch := range text[cookieStart:] {
		if ch == ';' {
			end = cookieStart + i
			break
		}
	}
	if end < 0 {
		return ""
	}
	return text[cookieStart:end]
}
