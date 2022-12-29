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
)

var _ = Describe("GetClusterState", func() {
	var (
		mockController          *gomock.Controller
		eksServiceMock          *mock_services.MockEKSServiceInterface
		getClusterStatusOptions *GetClusterStatusOpts
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		eksServiceMock = mock_services.NewMockEKSServiceInterface(mockController)
		getClusterStatusOptions = &GetClusterStatusOpts{
			EKSService: eksServiceMock,
			Config: &eksv1.EKSClusterConfig{
				Spec: eksv1.EKSClusterConfigSpec{
					DisplayName: "test-cluster",
				},
			},
		}
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should successfully get cluster state", func() {
		eksServiceMock.EXPECT().DescribeCluster(
			&eks.DescribeClusterInput{
				Name: aws.String(getClusterStatusOptions.Config.Spec.DisplayName),
			},
		).Return(nil, nil)
		_, err := GetClusterState(*getClusterStatusOptions)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should fail to get cluster state", func() {
		eksServiceMock.EXPECT().DescribeCluster(gomock.Any()).Return(nil, errors.New("error getting cluster state"))
		_, err := GetClusterState(*getClusterStatusOptions)
		Expect(err).To(HaveOccurred())
	})
})
