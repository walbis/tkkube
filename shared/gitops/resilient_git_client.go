package gitops

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	sharedconfig "shared-config/config"
	"shared-config/monitoring"
	"shared-config/resilience"
)

// GitOperation represents different types of Git operations
type GitOperation string

const (
	OpClone  GitOperation = "clone"
	OpPull   GitOperation = "pull"
	OpPush   GitOperation = "push"
	OpCommit GitOperation = "commit"
	OpAdd    GitOperation = "add"
	OpStatus GitOperation = "status"
	OpFetch  GitOperation = "fetch"
	OpBranch GitOperation = "branch"
	OpTag    GitOperation = "tag"
)

// ResilientGitClient provides Git operations with circuit breaker protection
type ResilientGitClient struct {
	config                *sharedconfig.GitOpsConfig
	circuitBreakerManager *resilience.CircuitBreakerManager
	monitoring            monitoring.MetricsCollector
	serviceName           string
	workingDir            string
}

// GitOperationResult represents the result of a Git operation
type GitOperationResult struct {
	Operation   GitOperation
	Success     bool
	Output      string
	Error       error
	Duration    time.Duration
	ExitCode    int
	StartTime   time.Time
	EndTime     time.Time
}

// GitRepositoryInfo contains information about a Git repository
type GitRepositoryInfo struct {
	URL           string
	Branch        string
	LocalPath     string
	LastCommit    string
	IsClean       bool
	RemoteStatus  string
	LastPull      time.Time
	LastPush      time.Time
}

// NewResilientGitClient creates a new resilient Git client
func NewResilientGitClient(
	config *sharedconfig.GitOpsConfig,
	circuitBreakerManager *resilience.CircuitBreakerManager,
	monitoring monitoring.MetricsCollector,
	workingDir string,
) *ResilientGitClient {
	if workingDir == "" {
		workingDir = "/tmp/gitops"
	}
	
	serviceName := "git"
	
	return &ResilientGitClient{
		config:                config,
		circuitBreakerManager: circuitBreakerManager,
		monitoring:            monitoring,
		serviceName:           serviceName,
		workingDir:            workingDir,
	}
}

// Clone clones a repository with circuit breaker protection
func (gc *ResilientGitClient) Clone(ctx context.Context, repoURL, localPath string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpClone, func() (*GitOperationResult, error) {
		// Ensure the parent directory exists
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %v", err)
		}
		
		args := []string{"clone", repoURL, localPath}
		
		// Add authentication if configured
		if gc.config != nil && gc.config.Repository.Auth.Method != "" {
			args = gc.addAuthArgs(args)
		}
		
		return gc.runGitCommand(ctx, OpClone, "", args...)
	})
}

// Pull pulls latest changes with circuit breaker protection
func (gc *ResilientGitClient) Pull(ctx context.Context, localPath string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpPull, func() (*GitOperationResult, error) {
		args := []string{"pull"}
		
		// Add authentication if configured
		if gc.config != nil && gc.config.Repository.Auth.Method != "" {
			args = gc.addAuthArgs(args)
		}
		
		return gc.runGitCommand(ctx, OpPull, localPath, args...)
	})
}

// Push pushes changes to remote with circuit breaker protection
func (gc *ResilientGitClient) Push(ctx context.Context, localPath string, branch string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpPush, func() (*GitOperationResult, error) {
		args := []string{"push", "origin"}
		if branch != "" {
			args = append(args, branch)
		}
		
		// Add authentication if configured
		if gc.config != nil && gc.config.Repository.Auth.Method != "" {
			args = gc.addAuthArgs(args)
		}
		
		return gc.runGitCommand(ctx, OpPush, localPath, args...)
	})
}

// Commit creates a commit with circuit breaker protection
func (gc *ResilientGitClient) Commit(ctx context.Context, localPath, message string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpCommit, func() (*GitOperationResult, error) {
		args := []string{"commit", "-m", message}
		return gc.runGitCommand(ctx, OpCommit, localPath, args...)
	})
}

// Add stages files for commit with circuit breaker protection
func (gc *ResilientGitClient) Add(ctx context.Context, localPath string, files ...string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpAdd, func() (*GitOperationResult, error) {
		args := append([]string{"add"}, files...)
		return gc.runGitCommand(ctx, OpAdd, localPath, args...)
	})
}

// AddAll stages all changes for commit with circuit breaker protection
func (gc *ResilientGitClient) AddAll(ctx context.Context, localPath string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpAdd, func() (*GitOperationResult, error) {
		args := []string{"add", "."}
		return gc.runGitCommand(ctx, OpAdd, localPath, args...)
	})
}

// Status gets repository status with circuit breaker protection
func (gc *ResilientGitClient) Status(ctx context.Context, localPath string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpStatus, func() (*GitOperationResult, error) {
		args := []string{"status", "--porcelain"}
		return gc.runGitCommand(ctx, OpStatus, localPath, args...)
	})
}

// Fetch fetches from remote with circuit breaker protection
func (gc *ResilientGitClient) Fetch(ctx context.Context, localPath string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpFetch, func() (*GitOperationResult, error) {
		args := []string{"fetch"}
		
		// Add authentication if configured
		if gc.config != nil && gc.config.Repository.Auth.Method != "" {
			args = gc.addAuthArgs(args)
		}
		
		return gc.runGitCommand(ctx, OpFetch, localPath, args...)
	})
}

// CreateBranch creates a new branch with circuit breaker protection
func (gc *ResilientGitClient) CreateBranch(ctx context.Context, localPath, branchName string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpBranch, func() (*GitOperationResult, error) {
		args := []string{"checkout", "-b", branchName}
		return gc.runGitCommand(ctx, OpBranch, localPath, args...)
	})
}

// CheckoutBranch switches to a branch with circuit breaker protection
func (gc *ResilientGitClient) CheckoutBranch(ctx context.Context, localPath, branchName string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpBranch, func() (*GitOperationResult, error) {
		args := []string{"checkout", branchName}
		return gc.runGitCommand(ctx, OpBranch, localPath, args...)
	})
}

// Tag creates a tag with circuit breaker protection
func (gc *ResilientGitClient) Tag(ctx context.Context, localPath, tagName, message string) (*GitOperationResult, error) {
	return gc.executeGitOperation(ctx, OpTag, func() (*GitOperationResult, error) {
		args := []string{"tag", "-a", tagName, "-m", message}
		return gc.runGitCommand(ctx, OpTag, localPath, args...)
	})
}

// GetRepositoryInfo gets detailed repository information
func (gc *ResilientGitClient) GetRepositoryInfo(ctx context.Context, localPath string) (*GitRepositoryInfo, error) {
	info := &GitRepositoryInfo{
		LocalPath: localPath,
	}
	
	var err error
	
	// Get remote URL
	if result, err := gc.runGitCommand(ctx, OpStatus, localPath, "remote", "get-url", "origin"); err == nil {
		info.URL = strings.TrimSpace(result.Output)
	}
	
	// Get current branch
	if result, err := gc.runGitCommand(ctx, OpStatus, localPath, "branch", "--show-current"); err == nil {
		info.Branch = strings.TrimSpace(result.Output)
	}
	
	// Get last commit
	if result, err := gc.runGitCommand(ctx, OpStatus, localPath, "rev-parse", "HEAD"); err == nil {
		info.LastCommit = strings.TrimSpace(result.Output)
	}
	
	// Check if working directory is clean
	if result, err := gc.Status(ctx, localPath); err == nil {
		info.IsClean = strings.TrimSpace(result.Output) == ""
	}
	
	// Get remote status
	if result, err := gc.runGitCommand(ctx, OpStatus, localPath, "status", "-b", "--porcelain"); err == nil {
		lines := strings.Split(result.Output, "\n")
		if len(lines) > 0 {
			info.RemoteStatus = strings.TrimSpace(lines[0])
		}
	}
	
	return info, err
}

// EnsureRepository ensures repository is cloned and up to date
func (gc *ResilientGitClient) EnsureRepository(ctx context.Context, repoURL, localPath, branch string) (*GitRepositoryInfo, error) {
	// Check if repository already exists
	if _, err := os.Stat(filepath.Join(localPath, ".git")); os.IsNotExist(err) {
		// Repository doesn't exist, clone it
		if _, err := gc.Clone(ctx, repoURL, localPath); err != nil {
			return nil, fmt.Errorf("failed to clone repository: %v", err)
		}
	}
	
	// Ensure we're on the correct branch
	if branch != "" {
		if _, err := gc.CheckoutBranch(ctx, localPath, branch); err != nil {
			// Branch might not exist, try to create it
			if _, err := gc.CreateBranch(ctx, localPath, branch); err != nil {
				return nil, fmt.Errorf("failed to checkout/create branch %s: %v", branch, err)
			}
		}
	}
	
	// Pull latest changes
	if _, err := gc.Pull(ctx, localPath); err != nil {
		return nil, fmt.Errorf("failed to pull latest changes: %v", err)
	}
	
	// Get repository info
	return gc.GetRepositoryInfo(ctx, localPath)
}

// CommitAndPush commits changes and pushes to remote
func (gc *ResilientGitClient) CommitAndPush(ctx context.Context, localPath, message, branch string) error {
	// Add all changes
	if _, err := gc.AddAll(ctx, localPath); err != nil {
		return fmt.Errorf("failed to add changes: %v", err)
	}
	
	// Check if there are changes to commit
	statusResult, err := gc.Status(ctx, localPath)
	if err != nil {
		return fmt.Errorf("failed to check status: %v", err)
	}
	
	if strings.TrimSpace(statusResult.Output) == "" {
		// No changes to commit
		return nil
	}
	
	// Commit changes
	if _, err := gc.Commit(ctx, localPath, message); err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}
	
	// Push to remote
	if _, err := gc.Push(ctx, localPath, branch); err != nil {
		return fmt.Errorf("failed to push changes: %v", err)
	}
	
	return nil
}

// HealthCheck performs a health check on Git operations
func (gc *ResilientGitClient) HealthCheck(ctx context.Context) error {
	// Simple health check by checking Git version
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	return gc.circuitBreakerManager.WrapGitOperation(healthCtx, func() error {
		cmd := exec.CommandContext(healthCtx, "git", "--version")
		return cmd.Run()
	})
}

// GetCircuitBreakerState returns the current circuit breaker state
func (gc *ResilientGitClient) GetCircuitBreakerState() resilience.CircuitBreakerState {
	cb := gc.circuitBreakerManager.GetServiceCircuitBreaker(resilience.ServiceGit)
	return cb.GetState()
}

// IsHealthy returns true if the circuit breaker is not in OPEN state
func (gc *ResilientGitClient) IsHealthy() bool {
	return gc.GetCircuitBreakerState() != resilience.StateOpen
}

// GetMetrics returns comprehensive metrics for Git operations
func (gc *ResilientGitClient) GetMetrics() map[string]interface{} {
	cb := gc.circuitBreakerManager.GetServiceCircuitBreaker(resilience.ServiceGit)
	cbMetrics := cb.GetMetrics()
	
	repoURL := ""
	if gc.config != nil {
		repoURL = gc.config.Repository.URL
	}
	
	return map[string]interface{}{
		"circuit_breaker": map[string]interface{}{
			"state":               cbMetrics.State.String(),
			"total_requests":      cbMetrics.TotalRequests,
			"successful_requests": cbMetrics.SuccessfulReqs,
			"failed_requests":     cbMetrics.FailedReqs,
			"rejected_requests":   cbMetrics.RejectedReqs,
			"failure_streak":      cbMetrics.FailureStreak,
			"last_failure":        cbMetrics.LastFailureTime,
			"last_success":        cbMetrics.LastSuccessTime,
		},
		"service_name":  gc.serviceName,
		"repository":    repoURL,
		"working_dir":   gc.workingDir,
		"timestamp":     time.Now(),
	}
}

// GetHealthStatus returns detailed health information
func (gc *ResilientGitClient) GetHealthStatus() map[string]interface{} {
	state := gc.GetCircuitBreakerState()
	metrics := gc.GetMetrics()
	
	cbMetrics := metrics["circuit_breaker"].(map[string]interface{})
	
	successRate := float64(0)
	if totalReqs := cbMetrics["total_requests"].(int64); totalReqs > 0 {
		successRate = float64(cbMetrics["successful_requests"].(int64)) / float64(totalReqs) * 100
	}
	
	repoURL := ""
	if gc.config != nil {
		repoURL = gc.config.Repository.URL
	}
	
	return map[string]interface{}{
		"service":        "git",
		"repository":     repoURL,
		"working_dir":    gc.workingDir,
		"healthy":        state != resilience.StateOpen,
		"state":          state.String(),
		"success_rate":   successRate,
		"total_requests": cbMetrics["total_requests"],
		"recent_failures": cbMetrics["failure_streak"],
		"last_failure":   cbMetrics["last_failure"],
		"timestamp":      time.Now(),
	}
}

// ResetCircuitBreaker resets the circuit breaker for Git operations
func (gc *ResilientGitClient) ResetCircuitBreaker() error {
	return gc.circuitBreakerManager.ResetCircuitBreaker(gc.serviceName)
}

// ForceOpenCircuitBreaker forces the circuit breaker to open state
func (gc *ResilientGitClient) ForceOpenCircuitBreaker() error {
	return gc.circuitBreakerManager.ForceOpenCircuitBreaker(gc.serviceName)
}

// Private helper methods

func (gc *ResilientGitClient) executeGitOperation(ctx context.Context, operation GitOperation, gitFunc func() (*GitOperationResult, error)) (*GitOperationResult, error) {
	var result *GitOperationResult
	var err error
	
	cbError := gc.circuitBreakerManager.WrapGitOperation(ctx, func() error {
		result, err = gitFunc()
		if err != nil {
			return err
		}
		
		// Check if the Git operation was successful
		if result != nil && !result.Success {
			return fmt.Errorf("git operation failed: %s", result.Error)
		}
		
		return nil
	})
	
	if cbError != nil {
		gc.recordMetric(fmt.Sprintf("git_%s_errors", operation), 1)
		
		// Return a result object even for circuit breaker errors
		return &GitOperationResult{
			Operation: operation,
			Success:   false,
			Error:     cbError,
			StartTime: time.Now(),
			EndTime:   time.Now(),
		}, cbError
	}
	
	gc.recordMetric(fmt.Sprintf("git_%s_operations", operation), 1)
	if result != nil && result.Duration > 0 {
		gc.recordDuration(fmt.Sprintf("git_%s_duration", operation), result.Duration)
	}
	
	return result, nil
}

func (gc *ResilientGitClient) runGitCommand(ctx context.Context, operation GitOperation, workDir string, args ...string) (*GitOperationResult, error) {
	start := time.Now()
	
	result := &GitOperationResult{
		Operation: operation,
		StartTime: start,
	}
	
	// Apply timeout from configuration
	timeout := 5 * time.Minute // default
	if gc.config != nil {
		switch operation {
		case OpClone:
			timeout = time.Duration(gc.config.Structure.ArgoCD.Namespace) // Using a timeout field if available
		case OpPush, OpPull:
			timeout = 2 * time.Minute
		default:
			timeout = 1 * time.Minute
		}
	}
	
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	cmd := exec.CommandContext(cmdCtx, "git", args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	
	// Set up environment for authentication
	cmd.Env = gc.setupGitEnvironment()
	
	output, err := cmd.CombinedOutput()
	
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Output = string(output)
	result.Error = err
	
	if err != nil {
		result.Success = false
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		}
	} else {
		result.Success = true
		result.ExitCode = 0
	}
	
	return result, err
}

func (gc *ResilientGitClient) addAuthArgs(args []string) []string {
	if gc.config == nil || gc.config.Repository.Auth.Method == "" {
		return args
	}
	
	switch gc.config.Repository.Auth.Method {
	case "ssh":
		// SSH authentication is handled via environment variables
		return args
	case "pat", "token":
		// For HTTPS with token, the URL should include the token
		// This is typically handled at the URL level
		return args
	default:
		return args
	}
}

func (gc *ResilientGitClient) setupGitEnvironment() []string {
	env := os.Environ()
	
	if gc.config == nil || gc.config.Repository.Auth.Method == "" {
		return env
	}
	
	switch gc.config.Repository.Auth.Method {
	case "ssh":
		if gc.config.Repository.Auth.SSH.PrivateKeyPath != "" {
			env = append(env, fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no", gc.config.Repository.Auth.SSH.PrivateKeyPath))
		}
	case "pat", "token":
		if gc.config.Repository.Auth.PAT.Token != "" {
			// For some Git operations, we might need to set credentials
			env = append(env, fmt.Sprintf("GIT_ASKPASS=echo"))
			env = append(env, fmt.Sprintf("GIT_USERNAME=%s", gc.config.Repository.Auth.PAT.Username))
			env = append(env, fmt.Sprintf("GIT_PASSWORD=%s", gc.config.Repository.Auth.PAT.Token))
		}
	}
	
	return env
}

func (gc *ResilientGitClient) recordMetric(metricName string, value float64) {
	if gc.monitoring == nil {
		return
	}
	
	repoURL := ""
	if gc.config != nil {
		repoURL = gc.config.Repository.URL
	}
	
	labels := map[string]string{
		"service":    gc.serviceName,
		"repository": repoURL,
	}
	gc.monitoring.IncCounter(metricName, labels, value)
}

func (gc *ResilientGitClient) recordDuration(metricName string, duration time.Duration) {
	if gc.monitoring == nil {
		return
	}
	
	repoURL := ""
	if gc.config != nil {
		repoURL = gc.config.Repository.URL
	}
	
	labels := map[string]string{
		"service":    gc.serviceName,
		"repository": repoURL,
	}
	gc.monitoring.RecordDuration(metricName, labels, duration)
}

// Helper function to create resilient Git client from shared config
func NewResilientGitClientFromSharedConfig(
	sharedConfig *sharedconfig.SharedConfig,
	circuitBreakerManager *resilience.CircuitBreakerManager,
	monitoring monitoring.MetricsCollector,
	workingDir string,
) *ResilientGitClient {
	var gitOpsConfig *sharedconfig.GitOpsConfig
	if sharedConfig != nil {
		gitOpsConfig = &sharedConfig.GitOps
	}
	
	return NewResilientGitClient(gitOpsConfig, circuitBreakerManager, monitoring, workingDir)
}