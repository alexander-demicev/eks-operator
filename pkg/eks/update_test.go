package eks

import (
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	eksv1 "github.com/rancher/eks-operator/pkg/apis/eks.cattle.io/v1"
	"github.com/rancher/eks-operator/pkg/eks/services/mock_services"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("GetLaunchTemplateVersions", func() {
	var (
		mockController              *gomock.Controller
		eksServiceMock              *mock_services.MockEKSServiceInterface
		updateClusterVersionOptions *UpdateClusterVersionOpts
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		eksServiceMock = mock_services.NewMockEKSServiceInterface(mockController)
		updateClusterVersionOptions = &UpdateClusterVersionOpts{
			EKSService: eksServiceMock,
			Config: &eksv1.EKSClusterConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Spec: eksv1.EKSClusterConfigSpec{
					DisplayName:       "test-cluster",
					KubernetesVersion: aws.String("test1"),
				},
			},
			UpstreamClusterSpec: &eksv1.EKSClusterConfigSpec{
				KubernetesVersion: aws.String("test2"),
			},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should update cluster version", func() {
		eksServiceMock.EXPECT().UpdateClusterVersion(
			&eks.UpdateClusterVersionInput{
				Name:    aws.String(updateClusterVersionOptions.Config.Spec.DisplayName),
				Version: updateClusterVersionOptions.Config.Spec.KubernetesVersion,
			},
		).Return(nil, nil)
		updated, err := UpdateClusterVersion(*updateClusterVersionOptions)
		Expect(updated).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not update cluster version if version didn't change", func() {
		updateClusterVersionOptions.UpstreamClusterSpec.KubernetesVersion = aws.String("test1")
		updated, err := UpdateClusterVersion(*updateClusterVersionOptions)
		Expect(updated).To(BeFalse())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return error if update cluster version failed", func() {
		eksServiceMock.EXPECT().UpdateClusterVersion(gomock.Any()).Return(nil, errors.New("error updating cluster version"))
		updated, err := UpdateClusterVersion(*updateClusterVersionOptions)
		Expect(updated).To(BeFalse())
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("UpdateClusterTags", func() {
	var (
		mockController        *gomock.Controller
		eksServiceMock        *mock_services.MockEKSServiceInterface
		updateClusterTagsOpts *UpdateClusterTagsOpts
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		eksServiceMock = mock_services.NewMockEKSServiceInterface(mockController)
		updateClusterTagsOpts = &UpdateClusterTagsOpts{
			EKSService: eksServiceMock,
			ClusterARN: "test-cluster-arn",
			Config: &eksv1.EKSClusterConfig{
				Spec: eksv1.EKSClusterConfigSpec{
					Tags: map[string]string{
						"test1": "test1",
						"test2": "changed",
					},
				},
			},
			UpstreamClusterSpec: &eksv1.EKSClusterConfigSpec{
				Tags: map[string]string{
					"test1": "test1",
					"test2": "test2",
					"test3": "removed",
				},
			},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should update cluster tags", func() {
		eksServiceMock.EXPECT().TagResource(
			&eks.TagResourceInput{
				ResourceArn: aws.String(updateClusterTagsOpts.ClusterARN),
				Tags: map[string]*string{
					"test2": aws.String("changed"),
				},
			},
		).Return(nil, nil)
		eksServiceMock.EXPECT().UntagResource(
			&eks.UntagResourceInput{
				ResourceArn: aws.String(updateClusterTagsOpts.ClusterARN),
				TagKeys:     []*string{aws.String("test3")},
			},
		).Return(nil, nil)
		updated, err := UpdateClusterTags(*updateClusterTagsOpts)
		Expect(updated).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should only update changed tags", func() {
		updateClusterTagsOpts.UpstreamClusterSpec.Tags = map[string]string{
			"test1": "test1",
			"test2": "test2",
		}
		eksServiceMock.EXPECT().TagResource(
			&eks.TagResourceInput{
				ResourceArn: aws.String(updateClusterTagsOpts.ClusterARN),
				Tags: map[string]*string{
					"test2": aws.String("changed"),
				},
			},
		).Return(nil, nil)
		updated, err := UpdateClusterTags(*updateClusterTagsOpts)
		Expect(updated).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should only remove removed tags", func() {
		updateClusterTagsOpts.Config.Spec.Tags = map[string]string{
			"test1": "test1",
			"test2": "test2",
		}
		eksServiceMock.EXPECT().UntagResource(
			&eks.UntagResourceInput{
				ResourceArn: aws.String(updateClusterTagsOpts.ClusterARN),
				TagKeys:     []*string{aws.String("test3")},
			},
		).Return(nil, nil)
		updated, err := UpdateClusterTags(*updateClusterTagsOpts)
		Expect(updated).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not update cluster tags if tags didn't change", func() {
		updateClusterTagsOpts.UpstreamClusterSpec.Tags = map[string]string{
			"test1": "test1",
			"test2": "test2",
		}
		updateClusterTagsOpts.Config.Spec.Tags = map[string]string{
			"test1": "test1",
			"test2": "test2",
		}
		updated, err := UpdateClusterTags(*updateClusterTagsOpts)
		Expect(updated).To(BeFalse())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return error if update cluster tags failed", func() {
		eksServiceMock.EXPECT().TagResource(gomock.Any()).Return(nil, errors.New("error tagging resource"))
		updated, err := UpdateClusterTags(*updateClusterTagsOpts)
		Expect(updated).To(BeFalse())
		Expect(err).To(HaveOccurred())
	})

	It("should return error if untag cluster tags failed", func() {
		eksServiceMock.EXPECT().TagResource(gomock.Any()).Return(nil, nil)
		eksServiceMock.EXPECT().UntagResource(gomock.Any()).Return(nil, errors.New("error untagging resource"))
		updated, err := UpdateClusterTags(*updateClusterTagsOpts)
		Expect(updated).To(BeFalse())
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("UpdateLoggingTypes", func() {
	var (
		mockController         *gomock.Controller
		eksServiceMock         *mock_services.MockEKSServiceInterface
		updateLoggingTypesOpts *UpdateLoggingTypesOpts
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		eksServiceMock = mock_services.NewMockEKSServiceInterface(mockController)
		updateLoggingTypesOpts = &UpdateLoggingTypesOpts{
			EKSService: eksServiceMock,
			Config: &eksv1.EKSClusterConfig{
				Spec: eksv1.EKSClusterConfigSpec{
					LoggingTypes: []string{"test1", "test2", "test3-enabled"},
				},
			},
			UpstreamClusterSpec: &eksv1.EKSClusterConfigSpec{
				LoggingTypes: []string{"test1", "test2", "disabled"},
			},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should update cluster logging types", func() {
		eksServiceMock.EXPECT().UpdateClusterConfig(
			&eks.UpdateClusterConfigInput{
				Name: aws.String(updateLoggingTypesOpts.Config.Spec.DisplayName),
				Logging: &eks.Logging{
					ClusterLogging: []*eks.LogSetup{
						{
							Enabled: aws.Bool(false),
							Types:   []*string{aws.String("disabled")},
						},
						{
							Enabled: aws.Bool(true),
							Types:   []*string{aws.String("test3-enabled")},
						},
					},
				},
			},
		).Return(nil, nil)
		updated, err := UpdateClusterLoggingTypes(*updateLoggingTypesOpts)
		Expect(updated).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not update cluster logging types if logging types didn't change", func() {
		updateLoggingTypesOpts.UpstreamClusterSpec.LoggingTypes = []string{"test1", "test2", "test3-enabled"}
		updated, err := UpdateClusterLoggingTypes(*updateLoggingTypesOpts)
		Expect(updated).To(BeFalse())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return error if update cluster logging types failed", func() {
		eksServiceMock.EXPECT().UpdateClusterConfig(gomock.Any()).Return(nil, errors.New("error updating cluster config"))
		updated, err := UpdateClusterLoggingTypes(*updateLoggingTypesOpts)
		Expect(updated).To(BeFalse())
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("UpdateClusterAccess", func() {
	var (
		mockController          *gomock.Controller
		eksServiceMock          *mock_services.MockEKSServiceInterface
		updateClusterAccessOpts *UpdateClusterAccessOpts
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		eksServiceMock = mock_services.NewMockEKSServiceInterface(mockController)
		updateClusterAccessOpts = &UpdateClusterAccessOpts{
			EKSService: eksServiceMock,
			Config: &eksv1.EKSClusterConfig{
				Spec: eksv1.EKSClusterConfigSpec{
					PrivateAccess: aws.Bool(true),
					PublicAccess:  aws.Bool(true),
				},
			},
			UpstreamClusterSpec: &eksv1.EKSClusterConfigSpec{
				PrivateAccess: aws.Bool(false),
				PublicAccess:  aws.Bool(false),
			},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should update cluster access", func() {
		eksServiceMock.EXPECT().UpdateClusterConfig(
			&eks.UpdateClusterConfigInput{
				Name: aws.String(updateClusterAccessOpts.Config.Spec.DisplayName),
				ResourcesVpcConfig: &eks.VpcConfigRequest{
					EndpointPrivateAccess: aws.Bool(true),
					EndpointPublicAccess:  aws.Bool(true),
				},
			},
		).Return(nil, nil)
		updated, err := UpdateClusterAccess(*updateClusterAccessOpts)
		Expect(updated).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not update cluster access if access didn't change", func() {
		updateClusterAccessOpts.UpstreamClusterSpec.PrivateAccess = aws.Bool(true)
		updateClusterAccessOpts.UpstreamClusterSpec.PublicAccess = aws.Bool(true)
		updated, err := UpdateClusterAccess(*updateClusterAccessOpts)
		Expect(updated).To(BeFalse())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return error if update cluster access failed", func() {
		eksServiceMock.EXPECT().UpdateClusterConfig(gomock.Any()).Return(nil, errors.New("error updating cluster config"))
		updated, err := UpdateClusterAccess(*updateClusterAccessOpts)
		Expect(updated).To(BeFalse())
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("UpdateClusterPublicAccessSources", func() {
	var (
		mockController                       *gomock.Controller
		eksServiceMock                       *mock_services.MockEKSServiceInterface
		updateClusterPublicAccessSourcesOpts *UpdateClusterPublicAccessSourcesOpts
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		eksServiceMock = mock_services.NewMockEKSServiceInterface(mockController)
		updateClusterPublicAccessSourcesOpts = &UpdateClusterPublicAccessSourcesOpts{
			EKSService: eksServiceMock,
			Config: &eksv1.EKSClusterConfig{
				Spec: eksv1.EKSClusterConfigSpec{
					PublicAccessSources: []string{"test1", "test2"},
				},
			},
			UpstreamClusterSpec: &eksv1.EKSClusterConfigSpec{
				PublicAccessSources: []string{"test1"},
			},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should update cluster public access sources", func() {
		eksServiceMock.EXPECT().UpdateClusterConfig(
			&eks.UpdateClusterConfigInput{
				Name: aws.String(updateClusterPublicAccessSourcesOpts.Config.Spec.DisplayName),
				ResourcesVpcConfig: &eks.VpcConfigRequest{
					PublicAccessCidrs: []*string{aws.String("test1"), aws.String("test2")},
				},
			},
		).Return(nil, nil)
		updated, err := UpdateClusterPublicAccessSources(*updateClusterPublicAccessSourcesOpts)
		Expect(updated).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not update cluster public access sources if public access sources didn't change", func() {
		updateClusterPublicAccessSourcesOpts.UpstreamClusterSpec.PublicAccessSources = []string{"test1", "test2"}
		updated, err := UpdateClusterPublicAccessSources(*updateClusterPublicAccessSourcesOpts)
		Expect(updated).To(BeFalse())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return error if update cluster public access sources failed", func() {
		eksServiceMock.EXPECT().UpdateClusterConfig(gomock.Any()).Return(nil, errors.New("error updating cluster config"))
		updated, err := UpdateClusterPublicAccessSources(*updateClusterPublicAccessSourcesOpts)
		Expect(updated).To(BeFalse())
		Expect(err).To(HaveOccurred())
	})
})
