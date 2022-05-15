package util

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

type ClientWithCacheOpts struct {
	CacheDir *string
}

type ClientWithCache struct {
	*cloudformation.Client

	cacheDir string
}

// Create a new "cached" CloudFormation client
//
// The returned client will persistently store results of `DescribeChangeSet` in the specified
// `CacheDir` (if unset: the current directory)
func NewClientWithCache(svc *cloudformation.Client, opts *ClientWithCacheOpts) (*ClientWithCache, error) {
	var cacheDir string
	if opts == nil || opts.CacheDir == nil || *opts.CacheDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("cannot determine current working directory, %v", err)
		}
		cacheDir = cwd
	} else {
		cacheDir = *opts.CacheDir
	}

	// Create the cachedir if needed
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot make cache directory %q, %v", cacheDir, err)
	}

	return &ClientWithCache{svc, cacheDir}, nil
}

func (c *ClientWithCache) DescribeChangeSet(ctx context.Context, params *cloudformation.DescribeChangeSetInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error) {
	// TODO: Check whether we already have that particular changeset cached locally
	var cachedName string
	changeSetName := aws.ToString(params.ChangeSetName)
	if arn.IsARN(changeSetName) {
		// Full ARN, the name is the part after changeSet
		changeSetArn, err := arn.Parse(changeSetName)
		if err != nil {
			return nil, err
		}

		parts := strings.Split(changeSetArn.Resource, "/")
		if changeSetArn.Service != "cloudformation" || parts[0] != "changeSet" {
			return nil, fmt.Errorf("ARN %q is not referencing a CloudFormation changeset", changeSetName)
		}
		cachedName = fmt.Sprintf("%s/%s.json", c.cacheDir, parts[1])
	} else {
		cachedName = fmt.Sprintf("%s/%s.json", c.cacheDir, changeSetName)
	}

	cached, err := os.ReadFile(cachedName)
	if err == nil {
		result := &cloudformation.DescribeChangeSetOutput{}
		if json.Unmarshal(cached, result) == nil {
			return result, nil
		}
		// else: Query again
	}
	result, err := c.Client.DescribeChangeSet(ctx, params)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(result)
	if err == nil {
		// Marshalling worked, try to save the contents. If it didn't, there's no problem
		// and we just might get called again
		os.WriteFile(cachedName, data, 0644)
	}

	// Write. If it fails, bad luck.
	return result, nil
}
