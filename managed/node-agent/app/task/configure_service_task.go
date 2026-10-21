// Copyright (c) YugabyteDB, Inc.

package task

import (
	"context"
	"fmt"
	pb "node-agent/generated/service"
	"node-agent/util"
)

// PreflightCheckHandler implements task.AsyncTask.
type ConfigureServiceHandler struct {
	shellTask      *ShellTask
	param          *pb.ServiceConfig
	serviceEnabled bool
	result         string
}

// NewConfigureServiceHandler returns a new instance of ConfigureServiceHandler.
func NewConfigureServiceHandler(
	param *pb.ServiceConfig,
	enabled bool,
) *ConfigureServiceHandler {

	handler := &ConfigureServiceHandler{
		param:          param,
		serviceEnabled: enabled,
		shellTask:      nil,
	}
	return handler
}

// CurrentTaskStatus implements the AsyncTask method.
func (handler *ConfigureServiceHandler) CurrentTaskStatus() *TaskStatus {
	// We init eventual task only after we check provisioning status.
	if handler.shellTask == nil {
		return &TaskStatus{
			Info: util.NewBuffer(1),
		}
	}
	taskStatus := handler.shellTask.CurrentTaskStatus()
	taskStatus.Info = util.NewBuffer(1)
	return taskStatus
}

// String implements the AsyncTask method.
func (handler *ConfigureServiceHandler) String() string {
	return handler.shellTask.String()
}

func (handler *ConfigureServiceHandler) Handle(
	ctx context.Context,
) (*pb.DescribeTaskResponse, error) {
	ybHomeDir := handler.param.GetYbHomeDir()
	util.FileLogger().Debug(ctx, "Starting install/configure earlyoom handler.")
	checkProvisionedTask := NewShellTaskWithUser(
		"checkProvisioned",
		"yugabyte",
		util.DefaultShell,
		[]string{"-c", fmt.Sprintf("ls %s/bin/configure_earlyoom_service.sh", ybHomeDir)},
	)
	provisionedStatus, err := checkProvisionedTask.Process(ctx)
	earlyoomProvisioned := false
	if err != nil {
		util.FileLogger().
			Errorf(ctx, "Failed to check if earlyoom is installed - %s", err)
	} else {
		earlyoomProvisioned = (provisionedStatus.ExitStatus.Code == 0)
	}

	user, command, options := handler.getCommandWithOptions(ybHomeDir, earlyoomProvisioned)
	handler.shellTask = NewShellTaskWithUser(
		"runConfigureEarlyoomService",
		user,
		command,
		options,
	)

	util.FileLogger().
		Infof(ctx, "Starting install/configure earlyoom handler with %v", handler.shellTask.cmdInfo.RedactCommandArgs())

	output, err := handler.shellTask.Process(ctx)
	if err != nil {
		util.FileLogger().Errorf(ctx, "Configure earlyoom processing failed - %s", err.Error())
		return nil, err
	}
	data := output.Info.String()
	util.FileLogger().Debugf(ctx, "Configure earlyoom output data: %s", data)
	handler.result = "Completed"
	return &pb.DescribeTaskResponse{
		Data: &pb.DescribeTaskResponse_ConfigureServiceOutput{
			ConfigureServiceOutput: &pb.ConfigureServiceOutput{},
		},
	}, nil
}

func (handler *ConfigureServiceHandler) Result() string {
	return handler.result
}

// Returns the user, command and options to run.
// Enabling always goes through the installer so that the earlyoom binary on the node is
// replaced with the one bundled with this node-agent (older binaries may not support newer
// args, e.g. -K). Only disabling an already provisioned service uses the configure script.
func (handler *ConfigureServiceHandler) getCommandWithOptions(
	ybHomeDir string,
	earlyoomProvisioned bool,
) (string, string, []string) {
	if earlyoomProvisioned && !handler.serviceEnabled {
		command := fmt.Sprintf(
			"%s/bin/configure_earlyoom_service.sh",
			ybHomeDir,
		)
		return "yugabyte", command, []string{"-a", "disable"}
	}
	// Empty args keep the existing node config on reinstall (see earlyoom-installer.sh).
	args := handler.param.GetEarlyoomConfig().GetStartArgs()
	return "", util.EarlyoomScriptPath(), []string{
		"--enable", "--earlyoom_args", args, "--yb_home_dir", ybHomeDir,
	}
}
