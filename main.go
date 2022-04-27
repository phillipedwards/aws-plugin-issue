package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

/*
	To reproduce behavior ensure:
	- pulumi plugin install resource aws 4.38.1
	- pulumi plugin install resource aws 5.1.3
*/

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		cfg := config.New(ctx, "aws")
		awsProvider, err := aws.NewProvider(ctx, "aws", &aws.ProviderArgs{
			Region:  pulumi.String(cfg.Require("region")),
			Profile: pulumi.String(cfg.Require("profile")),
		})

		if err != nil {
			return nil
		}

		eksRole, err := iam.NewRole(ctx, "eks-iam-eksRole", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(`{
		    "Version": "2008-10-17",
		    "Statement": [{
		        "Sid": "",
		        "Effect": "Allow",
		        "Principal": {
		            "Service": "eks.amazonaws.com"
		        },
		        "Action": "sts:AssumeRole"
		    }]
		}`),
		}, pulumi.Provider(awsProvider))

		if err != nil {
			return err
		}

		eksPolicies := []string{
			"arn:aws:iam::aws:policy/AmazonEKSServicePolicy",
			"arn:aws:iam::aws:policy/AmazonEKSClusterPolicy",
		}

		for i, eksPolicy := range eksPolicies {
			_, err := iam.NewRolePolicyAttachment(ctx, fmt.Sprintf("rpa-%d", i), &iam.RolePolicyAttachmentArgs{
				PolicyArn: pulumi.String(eksPolicy),
				Role:      eksRole.Name,
			}, pulumi.Provider(awsProvider))
			if err != nil {
				return err
			}
		}

		cluster, err := eks.NewCluster(ctx, "example", &eks.ClusterArgs{
			RoleArn: eksRole.Arn,
			VpcConfig: &eks.ClusterVpcConfigArgs{
				SubnetIds: pulumi.StringArray{
					pulumi.String("your-subnetids"),
					pulumi.String("your-subnetids"),
				},
			},
		}, pulumi.Provider(awsProvider))
		if err != nil {
			return err
		}

		// Run an intermediate lookup on the cluster in order to extract config later.
		// with explicit provider
		// this will select the latest version of the plugin
		clusterLookedup := pulumi.All(cluster.Name, cluster.Arn).ApplyT(func(v []interface{}) (interface{}, error) {
			n := v[0].(string)
			_ = v[1].(string)

			c, err := eks.LookupCluster(ctx, &eks.LookupClusterArgs{Name: n}, pulumi.Provider(awsProvider))
			if err != nil {
				return nil, err
			}
			return c, nil
		})

		clusterCAData := clusterLookedup.ApplyT(func(v interface{}) string {
			c := v.(*eks.LookupClusterResult)
			return c.CertificateAuthority.Data
		}).(pulumi.StringOutput)

		// Run an intermediate lookup on the cluster in order to extract config later.
		// with default provider
		// this will select the correct plugin
		clusterLookedupDefault := pulumi.All(cluster.Name, cluster.Arn).ApplyT(func(v []interface{}) (interface{}, error) {
			n := v[0].(string)
			_ = v[1].(string)

			c, err := eks.LookupCluster(ctx, &eks.LookupClusterArgs{Name: n})
			if err != nil {
				return nil, err
			}
			return c, nil
		})

		clusterCADataDefault := clusterLookedupDefault.ApplyT(func(v interface{}) string {
			c := v.(*eks.LookupClusterResult)
			return c.CertificateAuthority.Data
		}).(pulumi.StringOutput)

		// with version 5.1.3 of pulumi aws plugin, this should be an empty output
		ctx.Export("clusterCAData", clusterCAData)

		// with version 4.38.1 of pulumi aws plugin, this should be CA data output
		ctx.Export("clusterCADataDefault", clusterCADataDefault)

		return nil
	})
}
