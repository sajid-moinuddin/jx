package amazon

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
)

// GetAccountID returns the current account ID
func GetAccountIDAndRegion() (string, string, error) {
	sess, region, err := NewAwsSession()
	if err != nil {
		return "", region, err
	}
	svc := sts.New(sess)

	input := &sts.GetCallerIdentityInput{}

	result, err := svc.GetCallerIdentity(input)
	if err != nil {
		return "", region, err
	}
	if result.Account != nil {
		return *result.Account, region, nil
	}
	return "", region, fmt.Errorf("Could not find the AWS Account ID!")
}

func NewAwsSession() (*session.Session, string, error) {
	config := aws.Config{
		Region: aws.String(ResolveRegion()),
	}
	sess, err := session.NewSession(&config)
	return sess, *config.Region, err
}

// GetContainerRegistryHost
func GetContainerRegistryHost() (string, error) {
	accountId, region, err := GetAccountIDAndRegion()
	if err != nil {
		return "", err
	}
	return accountId + ".dkr.ecr." + region + ".amazonaws.com", nil
}

// LazyCreateRegistry lazily creates the ECR registry if it does not already exist
func LazyCreateRegistry(orgName string, appName string) error {
	// strip any tag/version from the app name
	idx := strings.Index(appName, ":")
	if idx > 0 {
		appName = appName[0:idx]
	}
	repoName := appName
	if orgName != "" {
		repoName = orgName + "/" + appName
	}
	repoName = strings.ToLower(repoName)
	log.Infof("Let's ensure that we have an ECR repository for the docker image %s\n", util.ColorInfo(repoName))
	sess, _, err := NewAwsSession()
	if err != nil {
		return err
	}
	svc := ecr.New(sess)
	repoInput := &ecr.DescribeRepositoriesInput{
		RepositoryNames: []*string{
			aws.String(repoName),
		},
	}
	result, err := svc.DescribeRepositories(repoInput)
	if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != ecr.ErrCodeRepositoryNotFoundException {
		return err
	}
	for _, repo := range result.Repositories {
		name := repo.String()
		log.Infof("Found repository: %s\n", name)
		if name == repoName {
			return nil
		}
	}
	createRepoInput := &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repoName),
	}
	createResult, err := svc.CreateRepository(createRepoInput)
	if err != nil {
		return fmt.Errorf("Failed to create the ECR repository for %s due to: %s", repoName, err)
	}
	repo := createResult.Repository
	if repo != nil {
		u := repo.RepositoryUri
		if u != nil {
			log.Infof("Created ECR repository: %s\n", util.ColorInfo(*u))
		}
	}
	return nil
}
