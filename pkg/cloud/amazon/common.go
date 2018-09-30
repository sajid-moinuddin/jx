package amazon

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"os"
	"path"
)

const DefaultRegion = "us-west-2"

func NewAwsSession(profileOption string, regionOption string) (*session.Session, error) {
	config := aws.Config{}
	if regionOption != "" {
		config.Region = aws.String(regionOption)
	}
	if _, err := os.Stat(path.Join(os.Getenv("HOME"), ".aws", "credentials")); !os.IsNotExist(err) {
		config.Credentials = credentials.NewChainCredentials(
			[]credentials.Provider{
				&credentials.EnvProvider{},
				&credentials.SharedCredentialsProvider{Filename: "", Profile: profileOption},
			})
	}

	sessionOptions := session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            config,
	}

	if profileOption != "" {
		sessionOptions.Profile = profileOption
	}

	awsSession, err := session.NewSessionWithOptions(sessionOptions)

	if *awsSession.Config.Region == "" {
		awsSession.Config.Region = aws.String(DefaultRegion)
	}

	if err != nil {
		return nil, err
	}
	return awsSession, nil
}

func NewAwsSessionWithoutOptions() (*session.Session, error) {
	return NewAwsSession("", "")
}

func ResolveRegion(profileOption string, regionOption string) (string, error) {
	session, err := NewAwsSession(profileOption, regionOption)
	if err != nil {
		return "", err
	}
	return *session.Config.Region, nil
}

func ResolveRegionWithoutOptions() (string, error) {
	return ResolveRegion("", "")
}
