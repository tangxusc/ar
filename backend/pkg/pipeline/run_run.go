//go:build linux

package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// writeRuntimeSpecForRun 为流水线单步生成 OCI spec：挂载 /tasks 与 /current-task，进程参数来自 step。
func writeRuntimeSpecForRun(bundleDir string, image v1.Image, tasksDir, currentTaskDir string, step *PipelineStepState) error {
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return fmt.Errorf("创建 bundle 目录失败: %w", err)
	}

	cfg, err := image.ConfigFile()
	if err != nil {
		return fmt.Errorf("读取镜像配置失败: %w", err)
	}

	args := make([]string, 0)
	if strings.TrimSpace(step.Entrypoint) != "" {
		args = append(args, step.Entrypoint)
	} else {
		args = append(args, cfg.Config.Entrypoint...)
	}
	args = append(args, step.Args...)
	if len(args) == 0 {
		args = append(args, cfg.Config.Cmd...)
	}
	if len(args) == 0 {
		return fmt.Errorf("步骤 %s 缺少 entrypoint/args，且镜像无默认 cmd", step.Name)
	}

	env := append([]string{}, step.Env...)
	if !containsEnvKey(env, "PATH") {
		env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}

	cwd := cfg.Config.WorkingDir
	if strings.TrimSpace(cwd) == "" {
		cwd = "/"
	}

	spec := specs.Spec{
		Version: specs.Version,
		Process: &specs.Process{
			Terminal:        false,
			Args:            args,
			Env:             env,
			Cwd:             cwd,
			NoNewPrivileges: true,
		},
		Root: &specs.Root{
			Path:     "rootfs",
			Readonly: false,
		},
		Hostname: "ar-run",
		Mounts: []specs.Mount{
			{Destination: "/proc", Type: "proc", Source: "proc"},
			{Destination: "/dev", Type: "tmpfs", Source: "tmpfs", Options: []string{"nosuid", "strictatime", "mode=755", "size=65536k"}},
			{Destination: "/dev/pts", Type: "devpts", Source: "devpts", Options: []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"}},
			{Destination: "/dev/shm", Type: "tmpfs", Source: "shm", Options: []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"}},
			{Destination: "/dev/mqueue", Type: "mqueue", Source: "mqueue", Options: []string{"nosuid", "noexec", "nodev"}},
			{Destination: "/sys", Type: "sysfs", Source: "sysfs", Options: []string{"nosuid", "noexec", "nodev", "ro"}},
			{Destination: "/tasks", Type: "bind", Source: tasksDir, Options: []string{"rbind", "rw"}},
			{Destination: "/current-task", Type: "bind", Source: currentTaskDir, Options: []string{"rbind", "rw"}},
		},
		Linux: &specs.Linux{
			Namespaces: []specs.LinuxNamespace{
				{Type: specs.PIDNamespace},
				{Type: specs.IPCNamespace},
				{Type: specs.UTSNamespace},
				{Type: specs.MountNamespace},
				{Type: specs.NetworkNamespace},
			},
			MaskedPaths: []string{
				"/proc/acpi", "/proc/asound", "/proc/kcore", "/proc/keys", "/proc/latency_stats",
				"/proc/timer_list", "/proc/timer_stats", "/proc/sched_debug", "/sys/firmware", "/proc/scsi",
			},
			ReadonlyPaths: []string{"/proc/bus", "/proc/fs", "/proc/irq", "/proc/sys", "/proc/sysrq-trigger"},
		},
	}

	specBytes, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 OCI spec 失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "config.json"), specBytes, 0644); err != nil {
		return fmt.Errorf("写入 config.json 失败: %w", err)
	}
	return nil
}

// RunStepResult 单步执行结果，供 RunPipeline 更新状态。
type RunStepResult struct {
	ExitCode int
	Err      error
}

// RunStep 运行流水线中的单步：从 store 取镜像、解包、写 spec（/tasks、/current-task）、执行容器。
func RunStep(ctx context.Context, runtimeRoot, imagesStoreDir, runDir, nodeDir, containerID string, step *PipelineStepState) RunStepResult {
	img, err := OpenImageFromStore(imagesStoreDir, step.Image)
	if err != nil {
		return RunStepResult{ExitCode: -1, Err: err}
	}

	bundleDir := filepath.Join(runDir, "bundles", step.Name)
	rootfsDir := filepath.Join(bundleDir, "rootfs")
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		return RunStepResult{ExitCode: -1, Err: fmt.Errorf("创建 rootfs 目录失败: %w", err)}
	}

	logrus.Infof("解包步骤镜像: %s -> %s", step.Name, rootfsDir)
	if err := extractRootfsFromImage(img, rootfsDir); err != nil {
		return RunStepResult{ExitCode: -1, Err: fmt.Errorf("解包步骤镜像失败: %w", err)}
	}
	if err := writeRuntimeSpecForRun(bundleDir, img, runDir, nodeDir, step); err != nil {
		return RunStepResult{ExitCode: -1, Err: err}
	}

	err = runOneShotContainer(ctx, runtimeRoot, bundleDir, containerID)
	if err != nil {
		// runOneShotContainer 在非 0 退出时返回 error，可解析或约定 ExitCode
		return RunStepResult{ExitCode: 1, Err: err}
	}
	return RunStepResult{ExitCode: 0}
}
