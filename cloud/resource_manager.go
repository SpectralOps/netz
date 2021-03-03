package cloud

import (
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/iam"

	log "github.com/cmpxchg16/netz/logger"
)

type ResourceManagerInterface interface {
	CreateResources(region string, numOfNic int, instanceType string, keyName string, securityGroup string, subnetId string, roleName string, rolePolicyName string, instanceProfileName string) error
	DestroyResources(skipDestroy bool)
}

type AWSResourceManager struct {
	ResourceManagerInterface cloudwatchLogsInterface
	networkInterfaces        []string
	allocationAddresses      []string
	instanceId               *string
	ecsCluster               *string
	region                   string
	guard                    sync.Mutex
}

func NewResourceManager() *AWSResourceManager {
	return &AWSResourceManager{}
}

func (rm *AWSResourceManager) ecsDeleteCluster(session *session.Session, clusterName string) error {
	svc := ecs.New(session)
	input := &ecs.DeleteClusterInput{
		Cluster: aws.String(clusterName),
	}

	result, err := svc.DeleteCluster(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return err
	}

	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) ecsCreateCluster(session *session.Session, clusterName string) error {
	svc := ecs.New(session)
	input := &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	}

	result, err := svc.CreateCluster(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return err
	}

	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) ecsListContainerInstances(session *session.Session, clusterName string) ([]*string, error) {
	svc := ecs.New(session)
	input := &ecs.ListContainerInstancesInput{
		Cluster: aws.String(clusterName),
	}

	result, err := svc.ListContainerInstances(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return nil, err
	}
	log.Logger.Trace(result)
	return result.ContainerInstanceArns, nil
}

func (rm *AWSResourceManager) ecsWaitForContainerInstances(session *session.Session, clusterName string) error {
	log.Logger.Info("waiting until ecs cluster will have container instances..")
	count := 1
	for {
		list, err := rm.ecsListContainerInstances(session, clusterName)
		if err != nil {
			return err
		}
		if list == nil || len(list) == 0 {
			time.Sleep(time.Second)
			log.Logger.Infof("still waiting (%d seconds)...", count)
		} else {
			log.Logger.Info("succeed, ecs cluster now have container instances")
			return nil
		}
		count++
		if count > 30 {
			return errors.New("too much time to wait for ecs container instances")
		}
	}
}

func (rm *AWSResourceManager) ec2TerminateInstance(session *session.Session, instanceId string) error {
	svc := ec2.New(session)
	input := &ec2.TerminateInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceId),
		},
	}

	result, err := svc.TerminateInstances(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return err
	}
	svc.WaitUntilInstanceTerminated(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(instanceId)},
	})
	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) ec2CreateInstance(session *session.Session, instanceType string, keyName string, securityGroup string, subnetId string, iamInstanceProfile string, ecsCluster string) (*string, error) {
	userdata := `
	#!/bin/bash
	echo ECS_CLUSTER=%s >> /etc/ecs/ecs.config
	`
	userdata = fmt.Sprintf(userdata, ecsCluster)
	userdata64 := base64.StdEncoding.EncodeToString([]byte(userdata))

	svc := ec2.New(session)

	input := &ec2.RunInstancesInput{
		ImageId:      aws.String("ami-02649d71054b25d22"),
		InstanceType: aws.String(instanceType),
		KeyName:      aws.String(keyName),
		MaxCount:     aws.Int64(1),
		MinCount:     aws.Int64(1),
		IamInstanceProfile: &ec2.IamInstanceProfileSpecification{
			Name: aws.String(iamInstanceProfile),
		},
		UserData: &userdata64,
		SecurityGroupIds: []*string{
			aws.String(securityGroup),
		},
		SubnetId: aws.String(subnetId),
	}

	result, err := svc.RunInstances(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return nil, err
	}
	rm.instanceId = result.Instances[0].InstanceId

	log.Logger.Info("wait until aws ec2 instance running..")
	svc.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(*result.Instances[0].InstanceId)},
	})
	log.Logger.Trace(result)
	return result.Instances[0].InstanceId, nil
}

func (rm *AWSResourceManager) iamPutRolePolicy(session *session.Session, roleName string, rolePolicyName string) error {
	rolePolicy := `{
		"Version": "2012-10-17",
		"Statement": [
		  {
			"Effect": "Allow",
			"Action": [
			  "ecr:BatchCheckLayerAvailability",
			  "ecr:BatchGetImage",
			  "ecr:DescribeRepositories",
			  "ecr:GetAuthorizationToken",
			  "ecr:GetDownloadUrlForLayer",
			  "ecr:GetRepositoryPolicy",
			  "ecr:ListImages",
			  "ecs:CreateCluster",
			  "ecs:DeregisterContainerInstance",
			  "ecs:DiscoverPollEndpoint",
			  "ecs:Poll",
			  "ecs:RegisterContainerInstance",
			  "ecs:StartTask",
			  "ecs:StartTelemetrySession",
			  "ecs:SubmitContainerStateChange",
			  "ecs:SubmitTaskStateChange",
			  "logs:DescribeLogGroups",
			  "logs:CreateLogGroup",
			  "logs:CreateLogStream",
			  "logs:DescribeLogGroups",
			  "logs:DescribeLogStreams",
              "logs:PutLogEvents",
              "logs:GetLogEvents",
			  "logs:FilterLogEvents",
			  "logs:PutRetentionPolicy"
			],
			"Resource": [
			  "*"
			]
		  }
		]
	  }
	`

	svc := iam.New(session)
	input := &iam.PutRolePolicyInput{
		PolicyDocument: aws.String(rolePolicy),
		PolicyName:     aws.String(rolePolicyName),
		RoleName:       aws.String(roleName),
	}

	result, err := svc.PutRolePolicy(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return err
	}

	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) iamCreateRole(session *session.Session, roleName string) error {
	ecsPolicy := `{
		"Version": "2012-10-17",
		"Statement": [
		  {
			"Effect": "Allow",
			"Principal": {
			  "Service": "ec2.amazonaws.com"
			},
			"Action": "sts:AssumeRole"
		  }
		]
	  }
	`

	svc := iam.New(session)
	input := &iam.CreateRoleInput{
		AssumeRolePolicyDocument: aws.String(ecsPolicy),
		RoleName:                 aws.String(roleName),
	}

	result, err := svc.CreateRole(input)
	ignore := false
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeEntityAlreadyExistsException:
				log.Logger.Info("iam role already exist")
				ignore = true
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		if ignore {
			return nil
		}
		return err
	}

	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) ec2CreateNetworkInterface(session *session.Session, securityGroup string, subnetId string) (*string, error) {
	svc := ec2.New(session)
	input := &ec2.CreateNetworkInterfaceInput{
		Description: aws.String("netz"),
		Groups: []*string{
			aws.String(securityGroup),
		},
		SubnetId: aws.String(subnetId),
	}

	result, err := svc.CreateNetworkInterface(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return nil, err
	}

	log.Logger.Trace(result)
	return result.NetworkInterface.NetworkInterfaceId, nil
}

func (rm *AWSResourceManager) ec2AttachNetworkInterface(session *session.Session, networkInterfaceId string, instanceId string, deviceIndex int64) error {
	svc := ec2.New(session)
	input := &ec2.AttachNetworkInterfaceInput{
		DeviceIndex:        aws.Int64(deviceIndex),
		InstanceId:         aws.String(instanceId),
		NetworkInterfaceId: aws.String(networkInterfaceId),
	}

	result, err := svc.AttachNetworkInterface(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return err
	}

	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) ec2AssociateIamInstanceProfile(session *session.Session, instanceId string, instanceProfileName string) error {
	svc := ec2.New(session)
	input := &ec2.AssociateIamInstanceProfileInput{
		IamInstanceProfile: &ec2.IamInstanceProfileSpecification{
			Name: aws.String(instanceProfileName),
		},
		InstanceId: aws.String(instanceId),
	}

	result, err := svc.AssociateIamInstanceProfile(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return err
	}
	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) iamAddRoleToInstanceProfile(session *session.Session, roleName string, instanceProfileName string) error {
	svc := iam.New(session)
	input := &iam.AddRoleToInstanceProfileInput{
		InstanceProfileName: aws.String(instanceProfileName),
		RoleName:            aws.String(roleName),
	}

	result, err := svc.AddRoleToInstanceProfile(input)
	ignore := false
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeEntityAlreadyExistsException:
				log.Logger.Warn("iam role already associated with instance profile")
				ignore = true
			case iam.ErrCodeLimitExceededException:
				ignore = true
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		if ignore {
			return nil
		}
		return err
	}

	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) iamCreateInstanceProfile(session *session.Session, instanceProfileName string) error {
	svc := iam.New(session)
	input := &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String(instanceProfileName),
	}

	result, err := svc.CreateInstanceProfile(input)
	ignore := false
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeEntityAlreadyExistsException:
				ignore = true
				log.Logger.Info("instance profile already exist")
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		if ignore {
			return nil
		}
		return err
	}

	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) ec2DeleteNetworkInterface(session *session.Session, networkInterfaceId string) error {
	svc := ec2.New(session)
	input := &ec2.DeleteNetworkInterfaceInput{
		NetworkInterfaceId: aws.String(networkInterfaceId),
	}

	result, err := svc.DeleteNetworkInterface(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return err
	}

	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) ec2ReleaseAddress(session *session.Session, allocationId string) error {
	svc := ec2.New(session)
	input := &ec2.ReleaseAddressInput{
		AllocationId: aws.String(allocationId),
	}

	result, err := svc.ReleaseAddress(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return err
	}

	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) ec2AllocateAddress(session *session.Session) (*string, error) {
	svc := ec2.New(session)
	input := &ec2.AllocateAddressInput{
		Domain: aws.String("vpc"),
	}

	result, err := svc.AllocateAddress(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return nil, err
	}

	log.Logger.Trace(result)
	return result.AllocationId, nil
}

func (rm *AWSResourceManager) ec2AssociateAddress(session *session.Session, allocationId string, networkInterfaceId string) error {
	svc := ec2.New(session)
	input := &ec2.AssociateAddressInput{
		AllocationId:       aws.String(allocationId),
		NetworkInterfaceId: aws.String(networkInterfaceId),
	}

	result, err := svc.AssociateAddress(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Logger.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Logger.Error(err.Error())
		}
		return err
	}

	log.Logger.Trace(result)
	return nil
}

func (rm *AWSResourceManager) CreateResources(region string, numOfNic int, instanceType string, keyName string, securityGroup string, subnetId string, roleName string, rolePolicyName string, instanceProfileName string, ecsCluster string) error {
	log.Logger.Info("going to create aws cloud resources")

	rm.region = region
	rm.ecsCluster = &ecsCluster

	session := session.New(&aws.Config{Region: aws.String(region)})
	log.Logger.Debug("aws going to create iam role")
	err := rm.iamCreateRole(session, roleName)
	if err != nil {
		return err
	}
	log.Logger.Info("aws iam role succeed")

	log.Logger.Debug("aws going to put role policy")
	err = rm.iamPutRolePolicy(session, roleName, rolePolicyName)
	if err != nil {
		return err
	}
	log.Logger.Info("aws put role policy succeed")

	log.Logger.Debug("aws going to create instance profile")
	err = rm.iamCreateInstanceProfile(session, instanceProfileName)
	if err != nil {
		return err
	}
	log.Logger.Info("aws instance profile succeed")

	log.Logger.Debug("aws going to add role to instance profile")
	err = rm.iamAddRoleToInstanceProfile(session, roleName, instanceProfileName)
	if err != nil {
		return err
	}
	log.Logger.Info("add iam to instance profile succeed")

	log.Logger.Debug("aws going to create ecs cluster")
	err = rm.ecsCreateCluster(session, ecsCluster)
	if err != nil {
		return err
	}
	log.Logger.Info("aws create ecs cluster succeed")

	log.Logger.Debug("aws going to create ec2 instance")
	instanceId, errInstance := rm.ec2CreateInstance(session, instanceType, keyName, securityGroup, subnetId, instanceProfileName, ecsCluster)
	if errInstance != nil {
		return errInstance
	}
	log.Logger.Debug("aws create ec2 instance succeed")

	for i := 1; i <= numOfNic; i++ {
		log.Logger.Debugf("aws going to create network interface: #%d", i)
		networkInterfaceId, err1 := rm.ec2CreateNetworkInterface(session, securityGroup, subnetId)
		if err1 != nil {
			return err1
		}
		log.Logger.Infof("aws create network interface succeed: #%d", i)

		log.Logger.Debugf("aws going to allocate elastic ip: #%d", i)
		allocationId, err2 := rm.ec2AllocateAddress(session)
		if err2 != nil {
			return err2
		}
		log.Logger.Infof("aws allocate elastic ip succeed: #%d", i)

		rm.networkInterfaces = append(rm.networkInterfaces, *networkInterfaceId)
		rm.allocationAddresses = append(rm.allocationAddresses, *allocationId)

		log.Logger.Debugf("aws going to associate elastic ip to network interface: #%d", i)
		err3 := rm.ec2AssociateAddress(session, *allocationId, *networkInterfaceId)
		if err3 != nil {
			return err3
		}
		log.Logger.Infof("aws associate elastic ip to network interface succeed: #%d", i)

		log.Logger.Debugf("aws going to attach network interface to instance: #%d", i)
		err4 := rm.ec2AttachNetworkInterface(session, *networkInterfaceId, *instanceId, int64(i))
		if err4 != nil {
			return err4
		}
		log.Logger.Infof("aws attach network interface to instance succeed: #%d", i)
	}

	err = rm.ecsWaitForContainerInstances(session, ecsCluster)
	if err != nil {
		return err
	}

	return nil
}

func (rm *AWSResourceManager) DestroyResources(skipDestroy bool) {
	rm.guard.Lock()
	defer rm.guard.Unlock()
	if rm.instanceId == nil {
		return
	}
	if skipDestroy {
		log.Logger.Warn("skipping destroy resources as you asked")
		return
	}

	log.Logger.Warn("destroying resources, it could take a minute so please don't kill me...")

	session := session.New(&aws.Config{Region: aws.String(rm.region)})
	if rm.instanceId != nil {
		err := rm.ec2TerminateInstance(session, *rm.instanceId)
		if err != nil {
			log.Logger.Error("failed to terminate ec2 instance")
		}
		rm.instanceId = nil
	}

	for _, allocationId := range rm.allocationAddresses {
		err := rm.ec2ReleaseAddress(session, allocationId)
		if err != nil {
			log.Logger.Errorf("failed to release elastic ip with id: %s", allocationId)
		}
	}

	for _, networkInterfaceId := range rm.networkInterfaces {
		err := rm.ec2DeleteNetworkInterface(session, networkInterfaceId)
		if err != nil {
			log.Logger.Errorf("failed to delete network interface with id: %s", networkInterfaceId)
		}
	}

	if rm.ecsCluster != nil {
		err := rm.ecsDeleteCluster(session, *rm.ecsCluster)
		if err != nil {
			log.Logger.Errorf("failed to delete ecs cluster: %s", *rm.ecsCluster)
		}
		rm.ecsCluster = nil
	}
	log.Logger.Info("done to destroy resources.")
}
