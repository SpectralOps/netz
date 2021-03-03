package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/cmpxchg16/netz/cloud"
	log "github.com/cmpxchg16/netz/logger"

	"github.com/urfave/cli/v2"
)

var (
	Version         string
	resourceManager *cloud.AWSResourceManager
)

func main() {
	app := cli.NewApp()
	app.Name = "netz"
	app.Usage = "netz cloud runner"
	app.UsageText = "netz [options]"
	app.Version = Version

	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Show debugging information",
		},
		&cli.StringFlag{
			Name:     "file, f",
			Usage:    "Task definition file in JSON or YAML",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "cluster, c",
			Value: "netz",
			Usage: "ECS cluster name",
		},
		&cli.StringFlag{
			Name:  "log-group, l",
			Value: "netz-runner",
			Usage: "Cloudwatch Log Group Name to write logs to",
		},
		&cli.StringSliceFlag{
			Name:     "security-group",
			Usage:    "Security groups to launch task. Can be specified multiple times",
			Required: true,
		},
		&cli.StringSliceFlag{
			Name:     "subnet",
			Usage:    "Subnet to launch task.",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "region, r",
			Usage:    "AWS Region",
			Required: true,
		},
		&cli.IntFlag{
			Name:     "number-of-nic, o",
			Usage:    "Number of network interfaces to create and attach to instance.",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "instance-type, t",
			Usage:    "Instance type.",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "instance-key-name, k",
			Usage:    "Instance key name to for ssh.",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "role-name, rn",
			Value: "netzRole",
			Usage: "Role name for netz.",
		},
		&cli.StringFlag{
			Name:  "role-policy-name, rp",
			Value: "netzPolicy",
			Usage: "Role policy name for netz.",
		},
		&cli.StringFlag{
			Name:  "instance-profile-name, i",
			Value: "netzInstanceProfile",
			Usage: "Instance profile name to attach to instance.",
		},
		&cli.IntFlag{
			Name:  "task-timeout, tt",
			Usage: "Task timeout (in minutes), stop everything after that.",
			Value: 120,
		},
		&cli.BoolFlag{
			Name:  "skip-destroy, sd",
			Value: false,
			Usage: "Skip destroy of cloud resources when done.",
		},
	}

	app.Action = func(ctx *cli.Context) error {
		fmt.Println()

		if _, err := os.Stat(ctx.String("file")); err != nil {
			return cli.NewExitError(err, 1)
		}

		log.SetLogger(ctx.Bool("debug"))

		runner := cloud.NewRunner()
		runner.TaskDefinitionFile = ctx.String("file")
		runner.Cluster = ctx.String("cluster")
		runner.LogGroupName = ctx.String("log-group")
		runner.SecurityGroups = ctx.StringSlice("security-group")
		runner.Subnets = ctx.StringSlice("subnet")
		runner.NumOfNic = ctx.Int("number-of-nic")
		runner.InstanceType = ctx.String("instance-type")
		runner.KeyName = ctx.String("instance-key-name")
		runner.RoleName = ctx.String("role-name")
		runner.RolePolicyName = ctx.String("role-policy-name")
		runner.InstanceProfileName = ctx.String("instance-profile-name")
		runner.TaskTimeout = ctx.Int("task-timeout")
		runner.SkipDestroy = ctx.Bool("skip-destroy")

		if runner.Region == "" {
			runner.Region = ctx.String("region")
		}

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt)

		go func(destroyResources bool) {
			<-quit
			log.Logger.Warn("signal caught, exiting...")
			resourceManager.DestroyResources(destroyResources)
			os.Exit(0)
		}(runner.SkipDestroy)

		resourceManager = cloud.NewResourceManager()
		err := resourceManager.CreateResources(runner.Region, runner.NumOfNic, runner.InstanceType, runner.KeyName, runner.SecurityGroups[0], runner.Subnets[0], runner.RoleName, runner.RolePolicyName, runner.InstanceProfileName, runner.Cluster)
		if err != nil {
			log.Logger.Error(err.Error())
			resourceManager.DestroyResources(runner.SkipDestroy)
			os.Exit(1)
		}

		if err := runner.Run(context.Background(), runner.TaskTimeout); err != nil {
			if ec, ok := err.(cli.ExitCoder); ok {
				return ec
			}
			log.Logger.Error(err.Error())
			resourceManager.DestroyResources(runner.SkipDestroy)
			os.Exit(1)
		}

		resourceManager.DestroyResources(runner.SkipDestroy)
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
}
