package scripts

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProductionBuildUsesPersistentGoCaches(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}

	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
	dockerfile := readNormalizedFile(t, filepath.Join(repositoryRoot, "Dockerfile"))
	deployScript := readNormalizedFile(t, filepath.Join(repositoryRoot, "scripts", "deploy.sh"))

	moduleCache := "--mount=type=cache,id=nebula-go-mod,target=/go/pkg/mod,sharing=locked"
	buildCache := "--mount=type=cache,id=nebula-go-build,target=/root/.cache/go-build,sharing=locked"

	if !strings.Contains(dockerfile, "RUN "+moduleCache+" go mod download") {
		t.Fatal("go mod download must use the persistent module cache")
	}
	if !strings.Contains(dockerfile, "RUN "+moduleCache+" "+buildCache+" test -n \"$VERSION\"") {
		t.Fatal("Go binary compilation must use the persistent module and build caches")
	}
	if !strings.Contains(deployScript, "export DOCKER_BUILDKIT=1") {
		t.Fatal("production deployment must explicitly enable BuildKit")
	}
}

func TestProductionDeploymentUsesDirectTunnelAndCompatibleSeccomp(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}

	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
	compose := readNormalizedFile(t, filepath.Join(repositoryRoot, "compose.production.yaml"))
	deployScript := readNormalizedFile(t, filepath.Join(repositoryRoot, "scripts", "deploy.sh"))
	ciDeployScript := readNormalizedFile(t, filepath.Join(repositoryRoot, "scripts", "ci-deploy.sh"))
	profilePath := filepath.Join(repositoryRoot, "deploy", "docker-default-seccomp-v26.1.4-enosys.json")
	profile := readNormalizedFile(t, profilePath)

	for _, content := range []string{compose, deployScript} {
		if strings.Contains(strings.ToLower(content), "mihomo") || strings.Contains(content, "127.0.0.1:7890") {
			t.Fatal("production deployment must not retain the removed Mihomo path")
		}
	}
	if !strings.Contains(compose, "networks: [edge]") || strings.Contains(compose, "network_mode: service:mihomo") {
		t.Fatal("cloudflared must connect directly through the production edge network")
	}
	if !strings.Contains(compose, "seccomp=/opt/nebula-api/deploy/docker-default-seccomp-v26.1.4-enosys.json") {
		t.Fatal("production postgres must use the versioned ENOSYS seccomp profile")
	}
	if !strings.Contains(deployScript, "Starting maintenance worker") {
		t.Fatal("production deployment must start the maintenance worker")
	}
	if !strings.Contains(deployScript, "Starting Matrix model catalog worker") {
		t.Fatal("production deployment must start the Matrix model catalog worker")
	}
	if !strings.Contains(ciDeployScript, `rm -rf -- '$remote_root'`) {
		t.Fatal("source synchronization must replace the deployment source directory")
	}
	if !strings.Contains(profile, `"defaultAction": "SCMP_ACT_ERRNO"`) || !strings.Contains(profile, `"defaultErrnoRet": 38`) {
		t.Fatal("seccomp profile must preserve syscall denial with ENOSYS compatibility")
	}
}

func readNormalizedFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	normalized := strings.ReplaceAll(string(contents), "\\\r\n", " ")
	normalized = strings.ReplaceAll(normalized, "\\\n", " ")
	return strings.Join(strings.Fields(normalized), " ")
}
