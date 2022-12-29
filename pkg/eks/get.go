package eks

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eks"
	eksv1 "github.com/rancher/eks-operator/pkg/apis/eks.cattle.io/v1"
	"github.com/rancher/eks-operator/pkg/eks/services"
)

type GetClusterStatusOpts struct {
	EKSService services.EKSServiceInterface
	Config     *eksv1.EKSClusterConfig
}

func GetClusterState(opts GetClusterStatusOpts) (*eks.DescribeClusterOutput, error) {
	return opts.EKSService.DescribeCluster(
		&eks.DescribeClusterInput{
			Name: aws.String(opts.Config.Spec.DisplayName),
		})
}
