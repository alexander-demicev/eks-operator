package eks

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/eks"
	eksv1 "github.com/rancher/eks-operator/pkg/apis/eks.cattle.io/v1"
	"github.com/rancher/eks-operator/pkg/eks/services"
	"github.com/rancher/eks-operator/templates"
	"github.com/rancher/eks-operator/utils"
)

const (
	launchTemplateNameFormat = "rancher-managed-lt-%s"
	launchTemplateTagKey     = "rancher-managed-template"
	launchTemplateTagValue   = "do-not-modify-or-delete"
	defaultStorageDeviceName = "/dev/xvda"
)

type CreateClusterOptions struct {
	EKSService services.EKSServiceInterface
	Config     *eksv1.EKSClusterConfig
	RoleARN    string
}

func CreateCluster(opts CreateClusterOptions) error {
	createClusterInput := newClusterInput(opts.Config, opts.RoleARN)

	_, err := opts.EKSService.CreateCluster(createClusterInput)
	return err
}

func newClusterInput(config *eksv1.EKSClusterConfig, roleARN string) *eks.CreateClusterInput {
	createClusterInput := &eks.CreateClusterInput{
		Name:    aws.String(config.Spec.DisplayName),
		RoleArn: aws.String(roleARN),
		ResourcesVpcConfig: &eks.VpcConfigRequest{
			EndpointPrivateAccess: config.Spec.PrivateAccess,
			EndpointPublicAccess:  config.Spec.PublicAccess,
			SecurityGroupIds:      aws.StringSlice(config.Status.SecurityGroups),
			SubnetIds:             aws.StringSlice(config.Status.Subnets),
			PublicAccessCidrs:     getPublicAccessCidrs(config.Spec.PublicAccessSources),
		},
		Tags:    getTags(config.Spec.Tags),
		Logging: getLogging(config.Spec.LoggingTypes),
		Version: config.Spec.KubernetesVersion,
	}

	if aws.BoolValue(config.Spec.SecretsEncryption) {
		createClusterInput.EncryptionConfig = []*eks.EncryptionConfig{
			{
				Provider: &eks.Provider{
					KeyArn: config.Spec.KmsKey,
				},
				Resources: aws.StringSlice([]string{"secrets"}),
			},
		}
	}

	return createClusterInput
}

type CreateStackOptions struct {
	CloudFormationService services.CloudFormationServiceInterface
	StackName             string
	DisplayName           string
	TemplateBody          string
	Capabilities          []string
	Parameters            []*cloudformation.Parameter
}

func CreateStack(opts CreateStackOptions) (*cloudformation.DescribeStacksOutput, error) {
	_, err := opts.CloudFormationService.CreateStack(&cloudformation.CreateStackInput{
		StackName:    aws.String(opts.StackName),
		TemplateBody: aws.String(opts.StackName),
		Capabilities: aws.StringSlice(opts.Capabilities),
		Parameters:   opts.Parameters,
		Tags: []*cloudformation.Tag{
			{
				Key:   aws.String("displayName"),
				Value: aws.String(opts.DisplayName),
			},
		},
	})
	if err != nil && !alreadyExistsInCloudFormationError(err) {
		return nil, fmt.Errorf("error creating master: %v", err)
	}

	var stack *cloudformation.DescribeStacksOutput
	status := "CREATE_IN_PROGRESS"

	for status == "CREATE_IN_PROGRESS" {
		time.Sleep(time.Second * 5)
		stack, err = opts.CloudFormationService.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: aws.String(opts.StackName),
		})
		if err != nil {
			return nil, fmt.Errorf("error polling stack info: %v", err)
		}

		status = *stack.Stacks[0].StackStatus
	}

	if len(stack.Stacks) == 0 {
		return nil, fmt.Errorf("stack did not have output: %v", err)
	}

	if status != "CREATE_COMPLETE" {
		reason := "reason unknown"
		events, err := opts.CloudFormationService.DescribeStackEvents(&cloudformation.DescribeStackEventsInput{
			StackName: aws.String(opts.StackName),
		})
		if err == nil {
			for _, event := range events.StackEvents {
				// guard against nil pointer dereference
				if event.ResourceStatus == nil || event.LogicalResourceId == nil || event.ResourceStatusReason == nil {
					continue
				}

				if *event.ResourceStatus == "CREATE_FAILED" {
					reason = *event.ResourceStatusReason
					break
				}

				if *event.ResourceStatus == "ROLLBACK_IN_PROGRESS" {
					reason = *event.ResourceStatusReason
					// do not break so that CREATE_FAILED takes priority
				}
			}
		}
		return nil, fmt.Errorf("stack failed to create: %v", reason)
	}

	return stack, nil
}

type CreateLaunchTemplateOptions struct {
	EC2Service services.EC2ServiceInterface
	Config     *eksv1.EKSClusterConfig
}

func CreateLaunchTemplate(opts CreateLaunchTemplateOptions) error {
	_, err := opts.EC2Service.DescribeLaunchTemplates(&ec2.DescribeLaunchTemplatesInput{
		LaunchTemplateIds: []*string{aws.String(opts.Config.Status.ManagedLaunchTemplateID)},
	})
	if opts.Config.Status.ManagedLaunchTemplateID == "" || doesNotExist(err) {
		lt, err := createLaunchTemplate(opts.EC2Service, opts.Config.Spec.DisplayName)
		if err != nil {
			return fmt.Errorf("error creating launch template: %w", err)
		}
		opts.Config.Status.ManagedLaunchTemplateID = aws.StringValue(lt.ID)
	} else if err != nil {
		return fmt.Errorf("error checking for existing launch template: %w", err)
	}

	return nil
}

func createLaunchTemplate(ec2Service services.EC2ServiceInterface, clusterDisplayName string) (*eksv1.LaunchTemplate, error) {
	// The first version of the rancher-managed launch template will be the default version.
	// Since the default version cannot be deleted until the launch template is deleted, it will not be used for any node group.
	// Also, launch templates cannot be created blank, so fake userdata is added to the first version.
	launchTemplateCreateInput := &ec2.CreateLaunchTemplateInput{
		LaunchTemplateData: &ec2.RequestLaunchTemplateData{UserData: aws.String("cGxhY2Vob2xkZXIK")},
		LaunchTemplateName: aws.String(fmt.Sprintf(launchTemplateNameFormat, clusterDisplayName)),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeLaunchTemplate),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(launchTemplateTagKey),
						Value: aws.String(launchTemplateTagValue),
					},
				},
			},
		},
	}

	awsLaunchTemplateOutput, err := ec2Service.CreateLaunchTemplate(launchTemplateCreateInput)
	if err != nil {
		return nil, err
	}

	return &eksv1.LaunchTemplate{
		Name:    awsLaunchTemplateOutput.LaunchTemplate.LaunchTemplateName,
		ID:      awsLaunchTemplateOutput.LaunchTemplate.LaunchTemplateId,
		Version: awsLaunchTemplateOutput.LaunchTemplate.LatestVersionNumber,
	}, nil
}

type CreateNodeGroupOptions struct {
	EC2Service            services.EC2ServiceInterface
	CloudFormationService services.CloudFormationServiceInterface
	EKSService            services.EKSServiceInterface

	Config    *eksv1.EKSClusterConfig
	NodeGroup eksv1.NodeGroup
}

func CreateNodeGroup(opts CreateNodeGroupOptions) (string, string, error) {
	var err error
	capacityType := eks.CapacityTypesOnDemand
	if aws.BoolValue(opts.NodeGroup.RequestSpotInstances) {
		capacityType = eks.CapacityTypesSpot
	}
	nodeGroupCreateInput := &eks.CreateNodegroupInput{
		ClusterName:   aws.String(opts.Config.Spec.DisplayName),
		NodegroupName: opts.NodeGroup.NodegroupName,
		Labels:        opts.NodeGroup.Labels,
		ScalingConfig: &eks.NodegroupScalingConfig{
			DesiredSize: opts.NodeGroup.DesiredSize,
			MaxSize:     opts.NodeGroup.MaxSize,
			MinSize:     opts.NodeGroup.MinSize,
		},
		CapacityType: aws.String(capacityType),
	}

	lt := opts.NodeGroup.LaunchTemplate

	if lt == nil {
		// In this case, the user has not specified their own launch template.
		// If the cluster doesn't have a launch template associated with it, then we create one.
		lt, err = createNewLaunchTemplateVersion(opts.EC2Service, opts.Config.Status.ManagedLaunchTemplateID, opts.NodeGroup)
		if err != nil {
			return "", "", err
		}
	}

	var launchTemplateVersion *string
	if aws.Int64Value(lt.Version) != 0 {
		launchTemplateVersion = aws.String(strconv.FormatInt(*lt.Version, 10))
	}

	nodeGroupCreateInput.LaunchTemplate = &eks.LaunchTemplateSpecification{
		Id:      lt.ID,
		Version: launchTemplateVersion,
	}

	if aws.BoolValue(opts.NodeGroup.RequestSpotInstances) {
		nodeGroupCreateInput.InstanceTypes = opts.NodeGroup.SpotInstanceTypes
	}

	if aws.StringValue(opts.NodeGroup.ImageID) == "" {
		if gpu := opts.NodeGroup.Gpu; aws.BoolValue(gpu) {
			nodeGroupCreateInput.AmiType = aws.String(eks.AMITypesAl2X8664Gpu)
		} else {
			nodeGroupCreateInput.AmiType = aws.String(eks.AMITypesAl2X8664)
		}
	}

	if len(opts.NodeGroup.Subnets) != 0 {
		nodeGroupCreateInput.Subnets = aws.StringSlice(opts.NodeGroup.Subnets)
	} else {
		nodeGroupCreateInput.Subnets = aws.StringSlice(opts.Config.Status.Subnets)
	}

	generatedNodeRole := opts.Config.Status.GeneratedNodeRole

	if aws.StringValue(opts.NodeGroup.NodeRole) == "" {
		if opts.Config.Status.GeneratedNodeRole == "" {
			finalTemplate := fmt.Sprintf(templates.NodeInstanceRoleTemplate, getEC2ServiceEndpoint(opts.Config.Spec.Region))
			output, err := CreateStack(CreateStackOptions{
				CloudFormationService: opts.CloudFormationService,
				StackName:             fmt.Sprintf("%s-node-instance-role", opts.Config.Spec.DisplayName),
				DisplayName:           opts.Config.Spec.DisplayName,
				TemplateBody:          finalTemplate,
				Capabilities:          []string{cloudformation.CapabilityCapabilityIam},
				Parameters:            []*cloudformation.Parameter{},
			})
			if err != nil {
				// If there was an error creating the node role stack, return an empty launch template
				// version and the error.
				return "", "", err
			}
			generatedNodeRole = getParameterValueFromOutput("NodeInstanceRole", output.Stacks[0].Outputs)
		}
		nodeGroupCreateInput.NodeRole = aws.String(generatedNodeRole)
	} else {
		nodeGroupCreateInput.NodeRole = opts.NodeGroup.NodeRole
	}

	_, err = opts.EKSService.CreateNodegroup(nodeGroupCreateInput)
	if err != nil {
		// If there was an error creating the node group, then the template version should be deleted
		// to prevent many launch template versions from being created before the issue is fixed.
		deleteLaunchTemplateVersions(opts.EC2Service, *lt.ID, []*string{launchTemplateVersion})
	}

	// Return the launch template version and generated node role to the calling function so they can
	// be set on the Status.
	return aws.StringValue(launchTemplateVersion), generatedNodeRole, err
}

func createNewLaunchTemplateVersion(ec2Service services.EC2ServiceInterface, launchTemplateID string, group eksv1.NodeGroup) (*eksv1.LaunchTemplate, error) {
	launchTemplate, err := buildLaunchTemplateData(ec2Service, group)
	if err != nil {
		return nil, err
	}

	launchTemplateVersionInput := &ec2.CreateLaunchTemplateVersionInput{
		LaunchTemplateData: launchTemplate,
		LaunchTemplateId:   aws.String(launchTemplateID),
	}

	awsLaunchTemplateOutput, err := ec2Service.CreateLaunchTemplateVersion(launchTemplateVersionInput)
	if err != nil {
		return nil, err
	}

	return &eksv1.LaunchTemplate{
		Name:    awsLaunchTemplateOutput.LaunchTemplateVersion.LaunchTemplateName,
		ID:      awsLaunchTemplateOutput.LaunchTemplateVersion.LaunchTemplateId,
		Version: awsLaunchTemplateOutput.LaunchTemplateVersion.VersionNumber,
	}, nil
}

func buildLaunchTemplateData(ec2Service services.EC2ServiceInterface, group eksv1.NodeGroup) (*ec2.RequestLaunchTemplateData, error) {
	var imageID *string
	if aws.StringValue(group.ImageID) != "" {
		imageID = group.ImageID
	}

	userdata := group.UserData
	if aws.StringValue(userdata) != "" {
		if !strings.Contains(*userdata, "Content-Type: multipart/mixed") {
			return nil, fmt.Errorf("userdata for nodegroup [%s] is not of mime time multipart/mixed", aws.StringValue(group.NodegroupName))
		}
		*userdata = base64.StdEncoding.EncodeToString([]byte(*userdata))
	}

	deviceName := aws.String(defaultStorageDeviceName)
	if aws.StringValue(group.ImageID) != "" {
		if rootDeviceName, err := getImageRootDeviceName(ec2Service, group.ImageID); err != nil {
			return nil, err
		} else if rootDeviceName != nil {
			deviceName = rootDeviceName
		}
	}

	launchTemplateData := &ec2.RequestLaunchTemplateData{
		ImageId:  imageID,
		KeyName:  group.Ec2SshKey,
		UserData: userdata,
		BlockDeviceMappings: []*ec2.LaunchTemplateBlockDeviceMappingRequest{
			{
				DeviceName: deviceName,
				Ebs: &ec2.LaunchTemplateEbsBlockDeviceRequest{
					VolumeSize: group.DiskSize,
				},
			},
		},
		TagSpecifications: utils.CreateTagSpecs(group.ResourceTags),
	}
	if !aws.BoolValue(group.RequestSpotInstances) {
		launchTemplateData.InstanceType = group.InstanceType
	}

	return launchTemplateData, nil
}

func getImageRootDeviceName(ec2Service services.EC2ServiceInterface, imageID *string) (*string, error) {
	if imageID == nil {
		return nil, fmt.Errorf("imageID is nil")
	}
	describeOutput, err := ec2Service.DescribeImages(&ec2.DescribeImagesInput{ImageIds: []*string{imageID}})
	if err != nil {
		return nil, err
	}
	if len(describeOutput.Images) == 0 {
		return nil, fmt.Errorf("no images returned for id %v", aws.StringValue(imageID))
	}

	return describeOutput.Images[0].RootDeviceName, nil
}

func getTags(tags map[string]string) map[string]*string {
	if len(tags) == 0 {
		return nil
	}

	return aws.StringMap(tags)
}

func getLogging(loggingTypes []string) *eks.Logging {
	if len(loggingTypes) == 0 {
		return &eks.Logging{
			ClusterLogging: []*eks.LogSetup{
				{
					Enabled: aws.Bool(false),
					Types:   aws.StringSlice(loggingTypes),
				},
			},
		}
	}
	return &eks.Logging{
		ClusterLogging: []*eks.LogSetup{
			{
				Enabled: aws.Bool(true),
				Types:   aws.StringSlice(loggingTypes),
			},
		},
	}
}

func getPublicAccessCidrs(publicAccessCidrs []string) []*string {
	if len(publicAccessCidrs) == 0 {
		return aws.StringSlice([]string{"0.0.0.0/0"})
	}

	return aws.StringSlice(publicAccessCidrs)
}

func alreadyExistsInCloudFormationError(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		case cloudformation.ErrCodeAlreadyExistsException:
			return true
		}
	}

	return false
}

func doesNotExist(err error) bool {
	// There is no better way of doing this because AWS API does not distinguish between a attempt to delete a stack
	// (or key pair) that does not exist, and, for example, a malformed delete request, so we have to parse the error
	// message
	if err != nil {
		return strings.Contains(err.Error(), "does not exist")
	}

	return false
}

func getEC2ServiceEndpoint(region string) string {
	if p, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), region); ok {
		return fmt.Sprintf("%s.%s", ec2.ServiceName, p.DNSSuffix())
	}
	return "ec2.amazonaws.com"
}

func getParameterValueFromOutput(key string, outputs []*cloudformation.Output) string {
	for _, output := range outputs {
		if *output.OutputKey == key {
			return *output.OutputValue
		}
	}

	return ""
}
