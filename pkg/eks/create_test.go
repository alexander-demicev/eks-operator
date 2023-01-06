package eks

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	eksv1 "github.com/rancher/eks-operator/pkg/apis/eks.cattle.io/v1"
	"github.com/rancher/eks-operator/pkg/eks/services/mock_services"
	"github.com/rancher/eks-operator/utils"
)

var _ = Describe("CreateCluster", func() {
	var (
		mockController        *gomock.Controller
		eksServiceMock        *mock_services.MockEKSServiceInterface
		clustercCreateOptions *CreateClusterOptions
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		eksServiceMock = mock_services.NewMockEKSServiceInterface(mockController)
		clustercCreateOptions = &CreateClusterOptions{
			EKSService: eksServiceMock,
			RoleARN:    "test",
			Config:     &eksv1.EKSClusterConfig{},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should successfully create a cluster", func() {
		eksServiceMock.EXPECT().CreateCluster(gomock.Any()).Return(nil, nil)
		Expect(CreateCluster(*clustercCreateOptions)).To(Succeed())
	})

	It("should fail to create a cluster", func() {
		eksServiceMock.EXPECT().CreateCluster(gomock.Any()).Return(nil, errors.New("error creating cluster"))
		Expect(CreateCluster(*clustercCreateOptions)).ToNot(Succeed())
	})
})

var _ = Describe("newClusterInput", func() {
	var (
		roleARN string
		config  *eksv1.EKSClusterConfig
	)

	BeforeEach(func() {
		roleARN = "test"
		config = &eksv1.EKSClusterConfig{
			Spec: eksv1.EKSClusterConfigSpec{
				DisplayName:         "test",
				PrivateAccess:       aws.Bool(true),
				PublicAccess:        aws.Bool(true),
				PublicAccessSources: []string{"test"},
				Tags:                map[string]string{"test": "test"},
				LoggingTypes:        []string{"test"},
				KubernetesVersion:   aws.String("test"),
				SecretsEncryption:   aws.Bool(true),
				KmsKey:              aws.String("test"),
			},
			Status: eksv1.EKSClusterConfigStatus{
				SecurityGroups: []string{"test"},
				Subnets:        []string{"test"},
			},
		}
	})

	It("should successfully create a cluster input", func() {
		clusterInput := newClusterInput(config, roleARN)
		Expect(clusterInput).ToNot(BeNil())

		Expect(clusterInput.Name).To(Equal(aws.String(config.Spec.DisplayName)))
		Expect(clusterInput.RoleArn).To(Equal(aws.String(roleARN)))
		Expect(clusterInput.ResourcesVpcConfig).ToNot(BeNil())
		Expect(clusterInput.ResourcesVpcConfig.SecurityGroupIds).To(Equal(aws.StringSlice(config.Status.SecurityGroups)))
		Expect(clusterInput.ResourcesVpcConfig.SubnetIds).To(Equal(aws.StringSlice(config.Status.Subnets)))
		Expect(clusterInput.ResourcesVpcConfig.EndpointPrivateAccess).To(Equal(config.Spec.PrivateAccess))
		Expect(clusterInput.ResourcesVpcConfig.EndpointPublicAccess).To(Equal(config.Spec.PublicAccess))
		Expect(clusterInput.ResourcesVpcConfig.PublicAccessCidrs).To(Equal(aws.StringSlice(config.Spec.PublicAccessSources)))
		Expect(clusterInput.Tags).To(Equal(aws.StringMap(config.Spec.Tags)))
		Expect(clusterInput.Logging.ClusterLogging).To(HaveLen(1))
		Expect(clusterInput.Logging.ClusterLogging[0].Enabled).To(Equal(aws.Bool(true)))
		Expect(clusterInput.Logging.ClusterLogging[0].Types).To(Equal(aws.StringSlice(config.Spec.LoggingTypes)))
		Expect(clusterInput.Version).To(Equal(config.Spec.KubernetesVersion))
		Expect(clusterInput.EncryptionConfig).To(HaveLen(1))
		Expect(clusterInput.EncryptionConfig[0].Provider.KeyArn).To(Equal(config.Spec.KmsKey))
		Expect(clusterInput.EncryptionConfig[0].Resources).To(Equal(aws.StringSlice([]string{"secrets"})))
	})

	It("should successfully create a cluster input with no public access cidrs set", func() {
		config.Spec.PublicAccessSources = []string{}
		clusterInput := newClusterInput(config, roleARN)
		Expect(clusterInput).ToNot(BeNil())

		Expect(clusterInput.ResourcesVpcConfig.PublicAccessCidrs).ToNot(BeNil())
		Expect(clusterInput.ResourcesVpcConfig.PublicAccessCidrs).To(Equal(aws.StringSlice([]string{"0.0.0.0/0"})))
	})

	It("should successfully create a cluster with no tags set", func() {
		config.Spec.Tags = map[string]string{}
		clusterInput := newClusterInput(config, roleARN)
		Expect(clusterInput).ToNot(BeNil())

		Expect(clusterInput.Tags).To(BeNil())
	})

	It("should successfully create a cluster with no logging types set", func() {
		config.Spec.LoggingTypes = []string{}
		clusterInput := newClusterInput(config, roleARN)
		Expect(clusterInput).ToNot(BeNil())

		Expect(clusterInput.Logging.ClusterLogging).To(HaveLen(1))
		Expect(clusterInput.Logging.ClusterLogging[0].Enabled).To(Equal(aws.Bool(false)))
		Expect(clusterInput.Logging.ClusterLogging[0].Types).To(Equal(aws.StringSlice(config.Spec.LoggingTypes)))
	})

	It("should successfully create a cluster with no secrets encryption set", func() {
		config.Spec.SecretsEncryption = aws.Bool(false)
		clusterInput := newClusterInput(config, roleARN)
		Expect(clusterInput).ToNot(BeNil())

		Expect(clusterInput.EncryptionConfig).To(BeNil())
	})
})

var _ = Describe("CreateStack", func() {
	var (
		mockController             *gomock.Controller
		cloudFormationsServiceMock *mock_services.MockCloudFormationServiceInterface
		stackCreationOptions       *CreateStackOptions
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		cloudFormationsServiceMock = mock_services.NewMockCloudFormationServiceInterface(mockController)
		stackCreationOptions = &CreateStackOptions{
			CloudFormationService: cloudFormationsServiceMock,
			StackName:             "test",
			DisplayName:           "test",
			TemplateBody:          "test",
			Capabilities:          []string{"test"},
			Parameters:            []*cloudformation.Parameter{{ParameterKey: aws.String("test"), ParameterValue: aws.String("test")}},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should successfully create a stack", func() {
		cloudFormationsServiceMock.EXPECT().CreateStack(&cloudformation.CreateStackInput{
			StackName:    &stackCreationOptions.StackName,
			TemplateBody: &stackCreationOptions.TemplateBody,
			Capabilities: aws.StringSlice(stackCreationOptions.Capabilities),
			Parameters:   stackCreationOptions.Parameters,
			Tags: []*cloudformation.Tag{
				{
					Key:   aws.String("displayName"),
					Value: aws.String(stackCreationOptions.DisplayName),
				},
			},
		}).Return(nil, nil)

		cloudFormationsServiceMock.EXPECT().DescribeStacks(
			&cloudformation.DescribeStacksInput{
				StackName: &stackCreationOptions.StackName,
			},
		).Return(
			&cloudformation.DescribeStacksOutput{
				Stacks: []*cloudformation.Stack{
					{
						StackStatus: aws.String("CREATE_COMPLETE"),
					},
				},
			}, nil)

		describeStacksOutput, err := CreateStack(*stackCreationOptions)
		Expect(err).ToNot(HaveOccurred())

		Expect(describeStacksOutput).ToNot(BeNil())
	})

	It("should fail to create a stack if CreateStack returns error", func() {
		cloudFormationsServiceMock.EXPECT().CreateStack(gomock.Any()).Return(nil, errors.New("error"))

		_, err := CreateStack(*stackCreationOptions)
		Expect(err).To(HaveOccurred())
	})

	It("should fail to create a stack if stack already exists", func() {
		cloudFormationsServiceMock.EXPECT().CreateStack(gomock.Any()).Return(nil, awserr.New(cloudformation.ErrCodeAlreadyExistsException, "", nil))
		cloudFormationsServiceMock.EXPECT().DescribeStacks(gomock.Any()).Return(
			&cloudformation.DescribeStacksOutput{
				Stacks: []*cloudformation.Stack{
					{
						StackStatus: aws.String("CREATE_COMPLETE"),
					},
				},
			}, nil)

		_, err := CreateStack(*stackCreationOptions)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should fail to create a stack if DescribeStack return errors returns error", func() {
		cloudFormationsServiceMock.EXPECT().CreateStack(gomock.Any()).Return(nil, nil)
		cloudFormationsServiceMock.EXPECT().DescribeStacks(gomock.Any()).Return(nil, errors.New("error"))

		_, err := CreateStack(*stackCreationOptions)
		Expect(err).To(HaveOccurred())
	})

	It("should fail to create a stack if stack status is CREATE_FAILED", func() {
		cloudFormationsServiceMock.EXPECT().CreateStack(gomock.Any()).Return(nil, nil)
		cloudFormationsServiceMock.EXPECT().DescribeStacks(gomock.Any()).Return(
			&cloudformation.DescribeStacksOutput{
				Stacks: []*cloudformation.Stack{
					{
						StackStatus: aws.String("CREATE_FAILED"),
					},
				},
			}, nil)
		cloudFormationsServiceMock.EXPECT().DescribeStackEvents(
			&cloudformation.DescribeStackEventsInput{
				StackName: &stackCreationOptions.StackName,
			},
		).Return(
			&cloudformation.DescribeStackEventsOutput{
				StackEvents: []*cloudformation.StackEvent{
					{
						ResourceStatus:       aws.String("CREATE_FAILED"),
						ResourceStatusReason: aws.String("CREATE_FAILED"),
						LogicalResourceId:    aws.String("test"),
					},
				},
			}, nil)

		_, err := CreateStack(*stackCreationOptions)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("CREATE_FAILED"))
	})

	It("should fail to create a stack if stack status is ROLLBACK_IN_PROGRESS", func() {
		cloudFormationsServiceMock.EXPECT().CreateStack(gomock.Any()).Return(nil, nil)
		cloudFormationsServiceMock.EXPECT().DescribeStacks(gomock.Any()).Return(
			&cloudformation.DescribeStacksOutput{
				Stacks: []*cloudformation.Stack{
					{
						StackStatus: aws.String("ROLLBACK_IN_PROGRESS"),
					},
				},
			}, nil)
		cloudFormationsServiceMock.EXPECT().DescribeStackEvents(
			&cloudformation.DescribeStackEventsInput{
				StackName: &stackCreationOptions.StackName,
			},
		).Return(
			&cloudformation.DescribeStackEventsOutput{
				StackEvents: []*cloudformation.StackEvent{
					{
						ResourceStatus:       aws.String("ROLLBACK_IN_PROGRESS"),
						ResourceStatusReason: aws.String("ROLLBACK_IN_PROGRESS"),
						LogicalResourceId:    aws.String("test"),
					},
				},
			}, nil)

		_, err := CreateStack(*stackCreationOptions)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ROLLBACK_IN_PROGRESS"))
	})
})

var _ = Describe("createLaunchTemplate", func() {
	var (
		mockController     *gomock.Controller
		ec2ServiceMock     *mock_services.MockEC2ServiceInterface
		clusterDisplayName = "testName"
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		ec2ServiceMock = mock_services.NewMockEC2ServiceInterface(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should create a launch template", func() {
		expectedOutput := &ec2.CreateLaunchTemplateOutput{
			LaunchTemplate: &ec2.LaunchTemplate{
				LaunchTemplateName:   aws.String("testName"),
				LaunchTemplateId:     aws.String("testID"),
				DefaultVersionNumber: aws.Int64(1),
			},
		}
		ec2ServiceMock.EXPECT().CreateLaunchTemplate(
			&ec2.CreateLaunchTemplateInput{
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
			},
		).Return(expectedOutput, nil)
		launchTemplate, err := createLaunchTemplate(ec2ServiceMock, clusterDisplayName)
		Expect(err).ToNot(HaveOccurred())
		Expect(launchTemplate).ToNot(BeNil())

		Expect(launchTemplate.Name).To(Equal(expectedOutput.LaunchTemplate.LaunchTemplateName))
		Expect(launchTemplate.ID).To(Equal(expectedOutput.LaunchTemplate.LaunchTemplateId))
		Expect(launchTemplate.Version).To(Equal(expectedOutput.LaunchTemplate.LatestVersionNumber))
	})

	It("should fail to create a launch template", func() {
		ec2ServiceMock.EXPECT().CreateLaunchTemplate(gomock.Any()).Return(nil, errors.New("error"))
		_, err := createLaunchTemplate(ec2ServiceMock, clusterDisplayName)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("CreateLaunchTemplate", func() {
	var (
		mockController           *gomock.Controller
		ec2ServiceMock           *mock_services.MockEC2ServiceInterface
		createLaunchTemplateOpts *CreateLaunchTemplateOptions
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		ec2ServiceMock = mock_services.NewMockEC2ServiceInterface(mockController)
		createLaunchTemplateOpts = &CreateLaunchTemplateOptions{
			EC2Service: ec2ServiceMock,
			Config: &eksv1.EKSClusterConfig{
				Spec: eksv1.EKSClusterConfigSpec{
					DisplayName: "test",
				},
				Status: eksv1.EKSClusterConfigStatus{
					ManagedLaunchTemplateID: "test",
				},
			},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should create a launch template if managed launch template ID is not set", func() {
		createLaunchTemplateOpts.Config.Status.ManagedLaunchTemplateID = ""
		ec2ServiceMock.EXPECT().CreateLaunchTemplate(gomock.Any()).Return(&ec2.CreateLaunchTemplateOutput{
			LaunchTemplate: &ec2.LaunchTemplate{
				LaunchTemplateName:   aws.String("testName"),
				LaunchTemplateId:     aws.String("testID"),
				DefaultVersionNumber: aws.Int64(1),
			},
		}, nil)

		ec2ServiceMock.EXPECT().DescribeLaunchTemplates(
			&ec2.DescribeLaunchTemplatesInput{
				LaunchTemplateIds: []*string{aws.String(createLaunchTemplateOpts.Config.Status.ManagedLaunchTemplateID)},
			},
		).Return(nil, nil)

		Expect(CreateLaunchTemplate(*createLaunchTemplateOpts)).To(Succeed())
		Expect(createLaunchTemplateOpts.Config.Status.ManagedLaunchTemplateID).To(Equal("testID"))
	})

	It("should create a launch template if managed launch template doesn't exist", func() {
		ec2ServiceMock.EXPECT().CreateLaunchTemplate(gomock.Any()).Return(&ec2.CreateLaunchTemplateOutput{
			LaunchTemplate: &ec2.LaunchTemplate{
				LaunchTemplateName:   aws.String("testName"),
				LaunchTemplateId:     aws.String("testID"),
				DefaultVersionNumber: aws.Int64(1),
			},
		}, nil)

		ec2ServiceMock.EXPECT().DescribeLaunchTemplates(
			&ec2.DescribeLaunchTemplatesInput{
				LaunchTemplateIds: []*string{aws.String(createLaunchTemplateOpts.Config.Status.ManagedLaunchTemplateID)},
			},
		).Return(nil, errors.New("does not exist"))

		Expect(CreateLaunchTemplate(*createLaunchTemplateOpts)).To(Succeed())
		Expect(createLaunchTemplateOpts.Config.Status.ManagedLaunchTemplateID).To(Equal("testID"))
	})

	It("should not create a launch template if managed launch template exists", func() {
		ec2ServiceMock.EXPECT().DescribeLaunchTemplates(
			&ec2.DescribeLaunchTemplatesInput{
				LaunchTemplateIds: []*string{aws.String(createLaunchTemplateOpts.Config.Status.ManagedLaunchTemplateID)},
			},
		).Return(nil, nil)

		Expect(CreateLaunchTemplate(*createLaunchTemplateOpts)).To(Succeed())
	})

	It("should fail to create a launch template", func() {
		ec2ServiceMock.EXPECT().DescribeLaunchTemplates(gomock.Any()).Return(nil, errors.New("error"))
		Expect(CreateLaunchTemplate(*createLaunchTemplateOpts)).ToNot(Succeed())
	})
})

var _ = Describe("getImageRootDeviceName", func() {
	var (
		mockController *gomock.Controller
		ec2ServiceMock *mock_services.MockEC2ServiceInterface
		imageID        = "test-image-id"
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		ec2ServiceMock = mock_services.NewMockEC2ServiceInterface(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should get the root device name", func() {
		exptectedRootDeviceName := "test-root-device-name"
		ec2ServiceMock.EXPECT().DescribeImages(
			&ec2.DescribeImagesInput{
				ImageIds: []*string{&imageID},
			},
		).Return(
			&ec2.DescribeImagesOutput{
				Images: []*ec2.Image{
					{
						RootDeviceName: &exptectedRootDeviceName,
					},
				},
			},
			nil)

		rootDeviceName, err := getImageRootDeviceName(ec2ServiceMock, &imageID)
		Expect(err).ToNot(HaveOccurred())

		Expect(rootDeviceName).To(Equal(&exptectedRootDeviceName))
	})

	It("should fail to get the root device name if image is nil", func() {
		_, err := getImageRootDeviceName(ec2ServiceMock, nil)
		Expect(err).To(HaveOccurred())
	})

	It("should fail to get the root device name if error is return by ec2", func() {
		ec2ServiceMock.EXPECT().DescribeImages(gomock.Any()).Return(nil, errors.New("error"))
		_, err := getImageRootDeviceName(ec2ServiceMock, &imageID)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("buildLaunchTemplateData", func() {
	var (
		mockController *gomock.Controller
		ec2ServiceMock *mock_services.MockEC2ServiceInterface
		group          *eksv1.NodeGroup
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		ec2ServiceMock = mock_services.NewMockEC2ServiceInterface(mockController)
		group = &eksv1.NodeGroup{
			ImageID:      aws.String("test-ami"),
			UserData:     aws.String("Content-Type: multipart/mixed ..."),
			DiskSize:     aws.Int64(20),
			ResourceTags: aws.StringMap(map[string]string{"test": "test"}),
			InstanceType: aws.String("test-instance-type"),
			Ec2SshKey:    aws.String("test-ssh-key"),
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should build a launch template data", func() {
		exptectedRootDeviceName := "test-root-device-name"
		ec2ServiceMock.EXPECT().DescribeImages(
			&ec2.DescribeImagesInput{
				ImageIds: []*string{group.ImageID},
			},
		).Return(
			&ec2.DescribeImagesOutput{
				Images: []*ec2.Image{
					{
						RootDeviceName: &exptectedRootDeviceName,
					},
				},
			},
			nil)

		launchTemplateData, err := buildLaunchTemplateData(ec2ServiceMock, *group)
		Expect(err).ToNot(HaveOccurred())

		Expect(launchTemplateData).ToNot(BeNil())
		Expect(launchTemplateData.ImageId).To(Equal(group.ImageID))
		Expect(launchTemplateData.KeyName).To(Equal(group.Ec2SshKey))
		Expect(launchTemplateData.UserData).To(Equal(group.UserData))
		Expect(launchTemplateData.BlockDeviceMappings).To(HaveLen(1))
		Expect(launchTemplateData.BlockDeviceMappings[0].DeviceName).To(Equal(&exptectedRootDeviceName))
		Expect(launchTemplateData.BlockDeviceMappings[0].Ebs.VolumeSize).To(Equal(group.DiskSize))
		Expect(launchTemplateData.TagSpecifications).To(Equal(utils.CreateTagSpecs(group.ResourceTags)))
		Expect(launchTemplateData.InstanceType).To(Equal(group.InstanceType))
	})

	It("should fail to build a launch template data if userdata is invalid", func() {
		group.UserData = aws.String("invalid-user-data")
		_, err := buildLaunchTemplateData(ec2ServiceMock, *group)
		Expect(err).To(HaveOccurred())
	})

	It("should fail to build a launch template data if error is return by ec2", func() {
		ec2ServiceMock.EXPECT().DescribeImages(gomock.Any()).Return(nil, errors.New("error"))
		_, err := buildLaunchTemplateData(ec2ServiceMock, *group)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("createNewLaunchTemplateVersion", func() {
	var (
		mockController *gomock.Controller
		ec2ServiceMock *mock_services.MockEC2ServiceInterface
		group          *eksv1.NodeGroup
		templateID     = "test-launch-template"
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		ec2ServiceMock = mock_services.NewMockEC2ServiceInterface(mockController)
		group = &eksv1.NodeGroup{
			DiskSize:     aws.Int64(20),
			ResourceTags: aws.StringMap(map[string]string{"test": "test"}),
			InstanceType: aws.String("test-instance-type"),
			Ec2SshKey:    aws.String("test-ssh-key"),
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should create a new launch template", func() {
		input, err := buildLaunchTemplateData(ec2ServiceMock, *group)
		Expect(err).ToNot(HaveOccurred())

		output := &ec2.CreateLaunchTemplateVersionOutput{
			LaunchTemplateVersion: &ec2.LaunchTemplateVersion{
				LaunchTemplateName: aws.String("test"),
				LaunchTemplateId:   aws.String("test"),
				VersionNumber:      aws.Int64(1),
			},
		}

		ec2ServiceMock.EXPECT().CreateLaunchTemplateVersion(&ec2.CreateLaunchTemplateVersionInput{
			LaunchTemplateData: input,
			LaunchTemplateId:   aws.String(templateID),
		}).Return(output, nil)

		launchTemplate, err := createNewLaunchTemplateVersion(ec2ServiceMock, templateID, *group)
		Expect(err).ToNot(HaveOccurred())

		Expect(launchTemplate.Name).To(Equal(output.LaunchTemplateVersion.LaunchTemplateName))
		Expect(launchTemplate.ID).To(Equal(output.LaunchTemplateVersion.LaunchTemplateId))
		Expect(launchTemplate.Version).To(Equal(output.LaunchTemplateVersion.VersionNumber))
	})

	It("should fail to create a new launch template if error is returned by ec2", func() {
		ec2ServiceMock.EXPECT().CreateLaunchTemplateVersion(gomock.Any()).Return(nil, errors.New("error"))
		_, err := createNewLaunchTemplateVersion(ec2ServiceMock, templateID, *group)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("CreateNodeGroup", func() {
	var (
		mockController      *gomock.Controller
		eksServiceMock      *mock_services.MockEKSServiceInterface
		ec2ServiceMock      *mock_services.MockEC2ServiceInterface
		cloudFormationMock  *mock_services.MockCloudFormationServiceInterface
		createNodeGroupOpts *CreateNodeGroupOptions
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		eksServiceMock = mock_services.NewMockEKSServiceInterface(mockController)
		ec2ServiceMock = mock_services.NewMockEC2ServiceInterface(mockController)
		cloudFormationMock = mock_services.NewMockCloudFormationServiceInterface(mockController)
		createNodeGroupOpts = &CreateNodeGroupOptions{
			EC2Service:            ec2ServiceMock,
			EKSService:            eksServiceMock,
			CloudFormationService: cloudFormationMock,

			Config:    &eksv1.EKSClusterConfig{},
			NodeGroup: eksv1.NodeGroup{},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should create a node group", func() {
		_, _, err := CreateNodeGroup(*createNodeGroupOpts)
		Expect(err).ToNot(HaveOccurred())

	})
})
