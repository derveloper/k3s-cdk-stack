package main

import (
	_ "embed"
	"fmt"
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awselasticloadbalancingv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"os"

	// "github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awselasticloadbalancingv2targets"
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

	sgControlPlane := awsec2.NewSecurityGroup(
		stack,
		jsii.String("k3s-security-group-cp"),
		&awsec2.SecurityGroupProps{
			Vpc:              vpc,
			AllowAllOutbound: jsii.Bool(true),
			Description:      jsii.String("K3S Security Group for Control Plane"),
		},
	)
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

	role := awsiam.NewRole(stack, jsii.String("k3s-role"), &awsiam.RoleProps{
		AssumedBy:       awsiam.NewServicePrincipal(jsii.String("ec2.amazonaws.com"), nil),
		ManagedPolicies: &[]awsiam.IManagedPolicy{awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("AmazonSSMManagedInstanceCore"))},
	})

	k3sControlPlaneUserData = fmt.Sprintf(k3sControlPlaneUserData, os.Getenv("K3S_TOKEN"))
	controlPlane := makeInstance(
		stack,
		"k3s-control-plane-01",
		vpc,
		role,
		sgControlPlane,
		k3sControlPlaneUserData,
		keyPair,
	)

	elb := awselasticloadbalancingv2.NewApplicationLoadBalancer(
		stack,
		jsii.String("k3s-control-plane-lb"),
		&awselasticloadbalancingv2.ApplicationLoadBalancerProps{
			Vpc:              vpc,
			InternetFacing:   jsii.Bool(true),
			LoadBalancerName: jsii.String("k3s-control-plane-lb"),
			SecurityGroup:    sgControlPlane,
		},
	)

	listener := elb.AddListener(
		jsii.String("k3s-control-plane-lb-listener"),
		&awselasticloadbalancingv2.BaseApplicationListenerProps{
			Port: jsii.Number(80),
		},
	)

	listener.AddTargets(
		jsii.String("k3s-control-plane-lb-listener-targets"),
		&awselasticloadbalancingv2.AddApplicationTargetsProps{
			Port:            jsii.Number(80),
			TargetGroupName: jsii.String("k3s-control-plane-01-tg"),
			Targets: &[]awselasticloadbalancingv2.IApplicationLoadBalancerTarget{
				awselasticloadbalancingv2targets.NewInstanceIdTarget(
					controlPlane.InstanceId(),
					jsii.Number(80),
				),
			},
		},
	)

	awscdk.NewCfnOutput(stack, jsii.String("controlPlaneInstanceId"), &awscdk.CfnOutputProps{
		Value:      controlPlane.InstanceId(),
		ExportName: jsii.String("controlPlaneInstanceId"),
	})
	awscdk.NewCfnOutput(stack, jsii.String("sgControlPlaneId"), &awscdk.CfnOutputProps{
		Value:      sgControlPlane.SecurityGroupId(),
		ExportName: jsii.String("sgControlPlaneId"),
	})

	sgAgents := awsec2.NewSecurityGroup(
		stack,
		jsii.String("k3s-security-group-agents"),
		&awsec2.SecurityGroupProps{
			Vpc:              vpc,
			AllowAllOutbound: jsii.Bool(true),
			Description:      jsii.String("K3S Security Group for agents"),
		},
	)
	sgAgents.AddIngressRule(
		awsec2.Peer_Ipv4(vpc.VpcCidrBlock()),
		awsec2.Port_AllTraffic(),
		jsii.String("Allow an inbound from VPC (IP v4)."),
		jsii.Bool(false),
	)

	k3sAgentUserData = fmt.Sprintf(k3sAgentUserData, os.Getenv("K3S_TOKEN"), *controlPlane.InstancePrivateIp())
	makeInstance(stack, "k3s-agent-01", vpc, nil, sgAgents, k3sAgentUserData, keyPair)
	makeInstance(stack, "k3s-agent-02", vpc, nil, sgAgents, k3sAgentUserData, keyPair)

	sgControlPlane.AddIngressRule(
		awsec2.Peer_SecurityGroupId(sgAgents.SecurityGroupId(), nil),
		awsec2.Port_Tcp(jsii.Number(6443)),
		jsii.String("Allow k3s api inbound (IP v4)."),
		jsii.Bool(false),
	)

	awscdk.Fn_ImportValue(jsii.String("controlPlanePublicIp"))

	return stack
}

func makeInstance(stack awscdk.Stack, instanceName string, vpc awsec2.Vpc, role awsiam.Role, sgControlPlane awsec2.SecurityGroup, userData string, keyPair awsec2.CfnKeyPair) awsec2.Instance {
	return awsec2.NewInstance(stack, jsii.String(instanceName), &awsec2.InstanceProps{
		InstanceType: awsec2.NewInstanceType(jsii.String("t3a.micro")),
		MachineImage: awsecs.EcsOptimizedImage_AmazonLinux2(
			awsecs.AmiHardwareType_STANDARD,
			&awsecs.EcsOptimizedImageOptions{},
		),
		Vpc:                       vpc,
		Role:                      role,
		SecurityGroup:             sgControlPlane,
		AllowAllOutbound:          jsii.Bool(true),
		DetailedMonitoring:        jsii.Bool(true),
		InstanceName:              jsii.String(instanceName),
		UserData:                  awsec2.MultipartUserData_Custom(&userData),
		UserDataCausesReplacement: jsii.Bool(true),
		KeyName:                   keyPair.KeyName(),
		VpcSubnets: &awsec2.SubnetSelection{
			SubnetType: awsec2.SubnetType_PRIVATE_WITH_EGRESS,
		},
	})
}

func NewSgRulesStack(scope constructs.Construct, id string, props *K3SCdkStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	sgControlPlaneId := awscdk.Fn_ImportValue(jsii.String("sgControlPlaneId"))

	sgControlPlane := awsec2.SecurityGroup_FromSecurityGroupId(
		stack,
		jsii.String("sgControlPlane"),
		sgControlPlaneId,
		&awsec2.SecurityGroupImportOptions{},
	)

	sgControlPlane.AddIngressRule(
		awsec2.Peer_SecurityGroupId(sgControlPlaneId, nil),
		awsec2.Port_Tcp(jsii.Number(6443)),
		jsii.String("Allow k3s api inbound (IP v4)."),
		jsii.Bool(false),
	)

	return stack
}

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	baseStack := NewK3SCdkStack(app, "K3SCdkStack", &K3SCdkStackProps{
		awscdk.StackProps{
			Env: env(),
		},
	})

	rulesStack := NewSgRulesStack(app, "K3SRulesStack", &K3SCdkStackProps{
		awscdk.StackProps{
			Env: env(),
		},
	})

	rulesStack.AddDependency(baseStack, jsii.String("sg rules depends on base stack"))

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
