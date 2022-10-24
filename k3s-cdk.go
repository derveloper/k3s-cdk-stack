package main

import (
	_ "embed"
	"fmt"
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"os"

	// "github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type K3SCdkStackProps struct {
	awscdk.StackProps
}

//go:embed user-data-cp.sh
var k3sControlPlaneUserData string

//go:embed user-data-agent.sh
var k3sAgentUserData string

//go:embed id_k3s.pub
var sshPubKey string

func NewK3SCdkStack(scope constructs.Construct, id string, props *K3SCdkStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	vpc := awsec2.NewVpc(stack, jsii.String("k3s-vpc"), &awsec2.VpcProps{
		NatGateways: jsii.Number(1),
		Cidr:        jsii.String("10.0.3.0/24"),
	})

	sgControlPlane := awsec2.NewSecurityGroup(stack, jsii.String("k3s-security-group-cp"), &awsec2.SecurityGroupProps{
		Vpc:              vpc,
		AllowAllOutbound: jsii.Bool(true),
		Description:      jsii.String("K3S Security Group for Control Plane"),
	})
	sgControlPlane.AddIngressRule(
		awsec2.Peer_AnyIpv4(),
		awsec2.Port_Tcp(jsii.Number(80)),
		jsii.String("Allow HTTP inbound (IP v4)."),
		jsii.Bool(false),
	)
	sgControlPlane.AddIngressRule(
		awsec2.Peer_AnyIpv4(),
		awsec2.Port_Tcp(jsii.Number(443)),
		jsii.String("Allow HTTPS inbound (IP v4)."),
		jsii.Bool(false),
	)
	sgControlPlane.AddIngressRule(
		awsec2.Peer_AnyIpv4(),
		awsec2.Port_Tcp(jsii.Number(22)),
		jsii.String("Allow HTTPS inbound (IP v4)."),
		jsii.Bool(false),
	)

	keyPair := awsec2.NewCfnKeyPair(stack, jsii.String("k3s-keypair"), &awsec2.CfnKeyPairProps{
		KeyName:           jsii.String("k3s-keypair"),
		KeyType:           jsii.String("ed25519"),
		PublicKeyMaterial: &sshPubKey,
	})

	k3sControlPlaneUserData = fmt.Sprintf(k3sControlPlaneUserData, os.Getenv("K3S_TOKEN"))
	controlPlane := awsec2.NewInstance(stack, jsii.String("k3s-control-plane-01"), &awsec2.InstanceProps{
		InstanceType:              awsec2.NewInstanceType(jsii.String("t3a.micro")),
		MachineImage:              awsecs.EcsOptimizedImage_AmazonLinux2(awsecs.AmiHardwareType_STANDARD, &awsecs.EcsOptimizedImageOptions{}),
		Vpc:                       vpc,
		SecurityGroup:             sgControlPlane,
		AllowAllOutbound:          jsii.Bool(true),
		DetailedMonitoring:        jsii.Bool(true),
		InstanceName:              jsii.String("k3s-control-plane"),
		UserData:                  awsec2.MultipartUserData_Custom(&k3sControlPlaneUserData),
		UserDataCausesReplacement: jsii.Bool(true),
		KeyName:                   keyPair.KeyName(),
		VpcSubnets: &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PUBLIC,
		},
	})

	ip := awsec2.NewCfnEIP(stack, jsii.String("controlPlaneElasticIp"), &awsec2.CfnEIPProps{
		InstanceId: controlPlane.InstanceId(),
	})

	controlPlane.AddUserData()

	awscdk.NewCfnOutput(stack, jsii.String("controlPlaneInstanceId"), &awscdk.CfnOutputProps{
		Value:      controlPlane.InstanceId(),
		ExportName: jsii.String("controlPlaneInstanceId"),
	})
	awscdk.NewCfnOutput(stack, jsii.String("controlPlanePublicIp"), &awscdk.CfnOutputProps{
		Value:      ip.Ref(),
		ExportName: jsii.String("controlPlanePublicIp"),
	})

	sgAgents := awsec2.NewSecurityGroup(stack, jsii.String("k3s-security-group-agents"), &awsec2.SecurityGroupProps{
		Vpc:              vpc,
		AllowAllOutbound: jsii.Bool(true),
		Description:      jsii.String("K3S Security Group for agents"),
	})
	sgAgents.AddIngressRule(
		awsec2.Peer_Ipv4(vpc.VpcCidrBlock()),
		awsec2.Port_AllTraffic(),
		jsii.String("Allow an inbound from VPC (IP v4)."),
		jsii.Bool(false),
	)

	k3sAgentUserData = fmt.Sprintf(k3sAgentUserData, os.Getenv("K3S_TOKEN"), *controlPlane.InstancePrivateIp())

	awsec2.NewInstance(stack, jsii.String("k3s-agent-01"), &awsec2.InstanceProps{
		InstanceType:              awsec2.NewInstanceType(jsii.String("t3a.micro")),
		MachineImage:              awsecs.EcsOptimizedImage_AmazonLinux2(awsecs.AmiHardwareType_STANDARD, &awsecs.EcsOptimizedImageOptions{}),
		Vpc:                       vpc,
		SecurityGroup:             sgAgents,
		AllowAllOutbound:          jsii.Bool(true),
		DetailedMonitoring:        jsii.Bool(true),
		InstanceName:              jsii.String("k3s-agent-01"),
		UserData:                  awsec2.MultipartUserData_Custom(&k3sAgentUserData),
		UserDataCausesReplacement: jsii.Bool(true),
		KeyName:                   keyPair.KeyName(),
		VpcSubnets: &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PRIVATE_WITH_EGRESS,
		},
	})

	awsec2.NewInstance(stack, jsii.String("k3s-agent-02"), &awsec2.InstanceProps{
		InstanceType:              awsec2.NewInstanceType(jsii.String("t3a.micro")),
		MachineImage:              awsecs.EcsOptimizedImage_AmazonLinux2(awsecs.AmiHardwareType_STANDARD, &awsecs.EcsOptimizedImageOptions{}),
		Vpc:                       vpc,
		SecurityGroup:             sgAgents,
		AllowAllOutbound:          jsii.Bool(true),
		DetailedMonitoring:        jsii.Bool(true),
		InstanceName:              jsii.String("k3s-agent-02"),
		UserData:                  awsec2.MultipartUserData_Custom(&k3sAgentUserData),
		UserDataCausesReplacement: jsii.Bool(true),
		KeyName:                   keyPair.KeyName(),
		VpcSubnets: &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PRIVATE_WITH_EGRESS,
		},
	})

	sgControlPlane.AddIngressRule(
		awsec2.Peer_SecurityGroupId(sgAgents.SecurityGroupId(), nil),
		awsec2.Port_Tcp(jsii.Number(6443)),
		jsii.String("Allow k3s api inbound (IP v4)."),
		jsii.Bool(false),
	)

	return stack
}

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	NewK3SCdkStack(app, "K3SCdkStack", &K3SCdkStackProps{
		awscdk.StackProps{
			Env: env(),
		},
	})

	app.Synth(nil)
}

// env determines the AWS environment (account+region) in which our stack is to
// be deployed. For more information see: https://docs.aws.amazon.com/cdk/latest/guide/environments.html
func env() *awscdk.Environment {
	return &awscdk.Environment{
		Account: jsii.String(os.Getenv("CDK_DEFAULT_ACCOUNT")),
		Region:  jsii.String(os.Getenv("CDK_DEFAULT_REGION")),
	}
}
