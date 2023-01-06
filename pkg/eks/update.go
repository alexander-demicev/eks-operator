package eks

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eks"
	eksv1 "github.com/rancher/eks-operator/pkg/apis/eks.cattle.io/v1"
	"github.com/rancher/eks-operator/pkg/eks/services"
	"github.com/rancher/eks-operator/utils"
	"github.com/sirupsen/logrus"
)

const (
	allOpen = "0.0.0.0/0"
)

type UpdateClusterVersionOpts struct {
	EKSService          services.EKSServiceInterface
	Config              *eksv1.EKSClusterConfig
	UpstreamClusterSpec *eksv1.EKSClusterConfigSpec
}

func UpdateClusterVersion(opts UpdateClusterVersionOpts) (bool, error) {
	updated := false
	if aws.StringValue(opts.UpstreamClusterSpec.KubernetesVersion) != aws.StringValue(opts.Config.Spec.KubernetesVersion) {
		logrus.Infof("updating kubernetes version for cluster [%s]", opts.Config.Name)
		_, err := opts.EKSService.UpdateClusterVersion(&eks.UpdateClusterVersionInput{
			Name:    aws.String(opts.Config.Spec.DisplayName),
			Version: opts.Config.Spec.KubernetesVersion,
		})
		if err != nil {
			return updated, fmt.Errorf("error updating cluster [%s] kubernetes version: %w", opts.Config.Name, err)
		}
		updated = true
	}

	return updated, nil
}

type UpdateClusterTagsOpts struct {
	EKSService          services.EKSServiceInterface
	Config              *eksv1.EKSClusterConfig
	UpstreamClusterSpec *eksv1.EKSClusterConfigSpec
	ClusterARN          string
}

func UpdateClusterTags(opts UpdateClusterTagsOpts) (bool, error) {
	updated := false
	if updateTags := utils.GetKeyValuesToUpdate(opts.Config.Spec.Tags, opts.UpstreamClusterSpec.Tags); updateTags != nil {
		_, err := opts.EKSService.TagResource(
			&eks.TagResourceInput{
				ResourceArn: aws.String(opts.ClusterARN),
				Tags:        updateTags,
			})
		if err != nil {
			return false, fmt.Errorf("error tagging cluster [%s]: %w", opts.Config.Name, err)
		}
		updated = true
	}

	if updateUntags := utils.GetKeysToDelete(opts.Config.Spec.Tags, opts.UpstreamClusterSpec.Tags); updateUntags != nil {
		_, err := opts.EKSService.UntagResource(
			&eks.UntagResourceInput{
				ResourceArn: aws.String(opts.ClusterARN),
				TagKeys:     updateUntags,
			})
		if err != nil {
			return false, fmt.Errorf("error untagging cluster [%s]: %w", opts.Config.Name, err)
		}
		updated = true
	}

	return updated, nil
}

type UpdateLoggingTypesOpts struct {
	EKSService          services.EKSServiceInterface
	Config              *eksv1.EKSClusterConfig
	UpstreamClusterSpec *eksv1.EKSClusterConfigSpec
}

func UpdateClusterLoggingTypes(opts UpdateLoggingTypesOpts) (bool, error) {
	updated := false
	if loggingTypesUpdate := getLoggingTypesUpdate(opts.Config.Spec.LoggingTypes, opts.UpstreamClusterSpec.LoggingTypes); loggingTypesUpdate != nil {
		_, err := opts.EKSService.UpdateClusterConfig(
			&eks.UpdateClusterConfigInput{
				Name:    aws.String(opts.Config.Spec.DisplayName),
				Logging: loggingTypesUpdate,
			},
		)
		if err != nil {
			return false, fmt.Errorf("error updating cluster [%s] logging types: %w", opts.Config.Name, err)
		}
		updated = true
	}

	return updated, nil
}

type UpdateClusterAccessOpts struct {
	EKSService          services.EKSServiceInterface
	Config              *eksv1.EKSClusterConfig
	UpstreamClusterSpec *eksv1.EKSClusterConfigSpec
}

func UpdateClusterAccess(opts UpdateClusterAccessOpts) (bool, error) {
	updated := false

	publicAccessUpdate := opts.Config.Spec.PublicAccess != nil && aws.BoolValue(opts.UpstreamClusterSpec.PublicAccess) != aws.BoolValue(opts.Config.Spec.PublicAccess)
	privateAccessUpdate := opts.Config.Spec.PrivateAccess != nil && aws.BoolValue(opts.UpstreamClusterSpec.PrivateAccess) != aws.BoolValue(opts.Config.Spec.PrivateAccess)
	if publicAccessUpdate || privateAccessUpdate {
		// public and private access updates need to be sent together. When they are sent one at a time
		// the request may be denied due to having both public and private access disabled.
		_, err := opts.EKSService.UpdateClusterConfig(
			&eks.UpdateClusterConfigInput{
				Name: aws.String(opts.Config.Spec.DisplayName),
				ResourcesVpcConfig: &eks.VpcConfigRequest{
					EndpointPublicAccess:  opts.Config.Spec.PublicAccess,
					EndpointPrivateAccess: opts.Config.Spec.PrivateAccess,
				},
			},
		)
		if err != nil {
			return false, fmt.Errorf("error updating cluster [%s] public/private access: %w", opts.Config.Name, err)
		}
		updated = true
	}

	return updated, nil
}

type UpdateClusterPublicAccessSourcesOpts struct {
	EKSService          services.EKSServiceInterface
	Config              *eksv1.EKSClusterConfig
	UpstreamClusterSpec *eksv1.EKSClusterConfigSpec
}

func UpdateClusterPublicAccessSources(opts UpdateClusterPublicAccessSourcesOpts) (bool, error) {
	updated := false
	// check public access CIDRs for update (public access sources)

	filteredSpecPublicAccessSources := filterPublicAccessSources(opts.Config.Spec.PublicAccessSources)
	filteredUpstreamPublicAccessSources := filterPublicAccessSources(opts.UpstreamClusterSpec.PublicAccessSources)
	if !utils.CompareStringSliceElements(filteredSpecPublicAccessSources, filteredUpstreamPublicAccessSources) {
		_, err := opts.EKSService.UpdateClusterConfig(
			&eks.UpdateClusterConfigInput{
				Name: aws.String(opts.Config.Spec.DisplayName),
				ResourcesVpcConfig: &eks.VpcConfigRequest{
					PublicAccessCidrs: getPublicAccessCidrs(opts.Config.Spec.PublicAccessSources),
				},
			},
		)
		if err != nil {
			return false, fmt.Errorf("error updating cluster [%s] public access sources: %w", opts.Config.Name, err)
		}

		updated = true
	}

	return updated, nil
}

func getLoggingTypesUpdate(loggingTypes []string, upstreamLoggingTypes []string) *eks.Logging {
	loggingUpdate := &eks.Logging{}

	if loggingTypesToDisable := getLoggingTypesToDisable(loggingTypes, upstreamLoggingTypes); loggingTypesToDisable != nil {
		loggingUpdate.ClusterLogging = append(loggingUpdate.ClusterLogging, loggingTypesToDisable)
	}

	if loggingTypesToEnable := getLoggingTypesToEnable(loggingTypes, upstreamLoggingTypes); loggingTypesToEnable != nil {
		loggingUpdate.ClusterLogging = append(loggingUpdate.ClusterLogging, loggingTypesToEnable)
	}

	if len(loggingUpdate.ClusterLogging) > 0 {
		return loggingUpdate
	}

	return nil
}

func getLoggingTypesToDisable(loggingTypes []string, upstreamLoggingTypes []string) *eks.LogSetup {
	loggingTypesMap := make(map[string]bool)

	for _, val := range loggingTypes {
		loggingTypesMap[val] = true
	}

	var loggingTypesToDisable []string
	for _, val := range upstreamLoggingTypes {
		if !loggingTypesMap[val] {
			loggingTypesToDisable = append(loggingTypesToDisable, val)
		}
	}

	if len(loggingTypesToDisable) > 0 {
		return &eks.LogSetup{
			Enabled: aws.Bool(false),
			Types:   aws.StringSlice(loggingTypesToDisable),
		}
	}

	return nil
}

func getLoggingTypesToEnable(loggingTypes []string, upstreamLoggingTypes []string) *eks.LogSetup {
	upstreamLoggingTypesMap := make(map[string]bool)

	for _, val := range upstreamLoggingTypes {
		upstreamLoggingTypesMap[val] = true
	}

	var loggingTypesToEnable []string
	for _, val := range loggingTypes {
		if !upstreamLoggingTypesMap[val] {
			loggingTypesToEnable = append(loggingTypesToEnable, val)
		}
	}

	if len(loggingTypesToEnable) > 0 {
		return &eks.LogSetup{
			Enabled: aws.Bool(true),
			Types:   aws.StringSlice(loggingTypesToEnable),
		}
	}

	return nil
}

func filterPublicAccessSources(sources []string) []string {
	if len(sources) == 0 {
		return nil
	}
	if len(sources) == 1 && sources[0] == allOpen {
		return nil
	}
	return sources
}
