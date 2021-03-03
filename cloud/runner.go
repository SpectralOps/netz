package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	log "github.com/cmpxchg16/netz/logger"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ecs"
)

func parse(file string) (*ecs.RegisterTaskDefinitionInput, error) {
	body, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var result ecs.RegisterTaskDefinitionInput
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

type Runner struct {
	TaskDefinitionFile  string
	Cluster             string
	LogGroupName        string
	Region              string
	Config              *aws.Config
	SecurityGroups      []string
	Subnets             []string
	NumOfNic            int
	InstanceType        string
	KeyName             string
	RoleName            string
	RolePolicyName      string
	InstanceProfileName string
	TaskTimeout         int
	SkipDestroy         bool
}

func NewRunner() *Runner {
	return &Runner{
		Region: os.Getenv("AWS_REGION"),
		Config: aws.NewConfig(),
	}
}

func (r *Runner) Run(ctx context.Context, taskTimeout int) error {
	taskDefinitionInput, err := parse(r.TaskDefinitionFile)
	if err != nil {
		return err
	}
	taskDefinitionInput.NetworkMode = aws.String(ecs.NetworkModeHost)
	log.Logger.Trace(taskDefinitionInput)

	streamPrefix := fmt.Sprintf("netz_task_%d", time.Now().Nanosecond())

	sess := session.Must(session.NewSession(r.Config.WithRegion(r.Region)))

	if err := createLogGroup(sess, r.LogGroupName); err != nil {
		return err
	}

	log.Logger.Infof("setting tasks to use log group %s", r.LogGroupName)
	for _, def := range taskDefinitionInput.ContainerDefinitions {
		def.LogConfiguration = &ecs.LogConfiguration{
			LogDriver: aws.String("awslogs"),
			Options: map[string]*string{
				"awslogs-group":         aws.String(r.LogGroupName),
				"awslogs-region":        aws.String(r.Region),
				"awslogs-stream-prefix": aws.String(streamPrefix),
			},
		}
	}

	taskDefinitionInput.ContainerDefinitions[0].Environment = append(taskDefinitionInput.ContainerDefinitions[0].Environment,
		&ecs.KeyValuePair{
			Name:  aws.String("TASK_DEFINITION"),
			Value: aws.String(streamPrefix),
		})

	svc := ecs.New(sess)

	log.Logger.Infof("registering a task for %s", *taskDefinitionInput.Family)
	resp, err := svc.RegisterTaskDefinition(taskDefinitionInput)
	if err != nil {
		return err
	}

	taskDefinition := fmt.Sprintf("%s:%d",
		*resp.TaskDefinition.Family, *resp.TaskDefinition.Revision)

	runTaskInput := &ecs.RunTaskInput{
		TaskDefinition: aws.String(taskDefinition),
		Cluster:        aws.String(r.Cluster),
		Count:          aws.Int64(1),
		Overrides: &ecs.TaskOverride{
			ContainerOverrides: []*ecs.ContainerOverride{},
		},
	}

	log.Logger.Infof("running task %s", taskDefinition)
	runResp, err := svc.RunTask(runTaskInput)
	if err != nil {
		return fmt.Errorf("unable to run task: %s", err.Error())
	}

	cwl := cloudwatchlogs.New(sess)

	for _, task := range runResp.Tasks {
		for _, container := range task.Containers {
			containerID := path.Base(*container.ContainerArn)
			watcher := &logWatcher{
				LogGroupName:   r.LogGroupName,
				LogStreamName:  logStreamName(streamPrefix, container, task),
				CloudWatchLogs: cwl,

				Printer: func(ev *cloudwatchlogs.FilteredLogEvent) bool {
					finishedPrefix := fmt.Sprintf(
						"container %s exited with",
						containerID,
					)
					if strings.HasPrefix(*ev.Message, finishedPrefix) {
						log.Logger.Infof("found container finished message for %s: %s",
							containerID, *ev.Message)
						return false
					}
					log.Logger.Info(*ev.Message)
					return true
				},
			}

			go func() {
				if err := watcher.Watch(ctx); err != nil {
					log.Logger.Tracef("log watcher returned error: %v", err)
				}
			}()
		}
	}

	var taskARNs []*string
	for _, task := range runResp.Tasks {
		log.Logger.Infof("waiting until task has stopped")
		taskARNs = append(taskARNs, task.TaskArn)
	}

	delay := time.Second * 10
	ctx, cancelFn := context.WithTimeout(aws.BackgroundContext(), time.Duration(taskTimeout)*time.Minute)
	defer cancelFn()

	err = svc.WaitUntilTasksStoppedWithContext(
		ctx,
		&ecs.DescribeTasksInput{
			Cluster: &r.Cluster,
			Tasks:   taskARNs,
		},
		request.WithWaiterDelay(request.ConstantWaiterDelay(delay)),
		request.WithWaiterMaxAttempts(0),
	)

	if err != nil {
		return err
	}

	log.Logger.Info("task was stopped")
	return nil
}

func logStreamName(logStreamPrefix string, container *ecs.Container, task *ecs.Task) string {
	return fmt.Sprintf(
		"%s/%s/%s",
		logStreamPrefix,
		*container.Name,
		path.Base(*task.TaskArn),
	)
}
