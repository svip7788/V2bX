package machine

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const defaultRepo = "svip7788/V2bX"

var (
	repoPattern    = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	versionPattern = regexp.MustCompile(`^[A-Za-z0-9_.+-]{1,64}$`)
	githubMirrors  = []string{
		"https://github.com/",
		"https://ghfast.top/https://github.com/",
		"https://gh-proxy.com/https://github.com/",
		"https://ghproxy.net/https://github.com/",
	}
)

func releaseArch() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "64", nil
	case "arm64":
		return "arm64-v8a", nil
	case "s390x":
		return "s390x", nil
	}
	return "", fmt.Errorf("当前架构不支持自动升级: %s", runtime.GOARCH)
}

// upgrade 返回是否需要重启、结论（过程经 progress 上报）
func (r *commandRunner) upgrade(cmd command) (bool, string, error) {
	var p struct {
		Version string `json:"version"`
		Repo    string `json:"repo"`
	}
	_ = json.Unmarshal(cmd.Params, &p)
	p.Version = strings.TrimSpace(p.Version)
	p.Repo = strings.TrimSpace(p.Repo)
	if p.Repo == "" {
		p.Repo = defaultRepo
	}
	logf := func(format string, args ...any) {
		r.progress(cmd.ID, fmt.Sprintf(">>> "+format, args...))
	}
	if !versionPattern.MatchString(p.Version) || p.Version == "latest" {
		return false, "", fmt.Errorf("版本号无效: %q", p.Version)
	}
	if !repoPattern.MatchString(p.Repo) {
		return false, "", fmt.Errorf("仓库格式无效: %q", p.Repo)
	}
	if cur := r.m.currentVersion(); cur == p.Version {
		return false, ">>> 当前已是 " + cur + "，无需升级", nil
	}
	arch, err := releaseArch()
	if err != nil {
		return false, "", err
	}

	stage, err := os.MkdirTemp("", "v2bx-upgrade-")
	if err != nil {
		return false, "", err
	}
	defer os.RemoveAll(stage)

	path := fmt.Sprintf("%s/releases/download/%s/V2bX-linux-%s.zip", p.Repo, p.Version, arch)
	zipFile := filepath.Join(stage, "V2bX.zip")
	downloaded := false
	for _, mirror := range githubMirrors {
		logf("下载: %s%s", mirror, path)
		if err := download(r.m.ctx, mirror+path, zipFile); err != nil {
			logf("失败: %v", err)
			continue
		}
		downloaded = true
		break
	}
	if !downloaded {
		return false, "", fmt.Errorf("所有下载源均失败")
	}

	extractDir := filepath.Join(stage, "files")
	if err := unzip(zipFile, extractDir); err != nil {
		return false, "", fmt.Errorf("解压失败: %w", err)
	}
	newBin := filepath.Join(extractDir, "V2bX")
	if err := os.Chmod(newBin, 0755); err != nil {
		return false, "", fmt.Errorf("压缩包中未找到 V2bX: %w", err)
	}
	ctx, cancel := context.WithTimeout(r.m.ctx, 15*time.Second)
	verOut, err := exec.CommandContext(ctx, newBin, "version").CombinedOutput()
	cancel()
	if err != nil {
		return false, "", fmt.Errorf("新版本无法运行: %v %s", err, truncateTail(string(verOut)))
	}
	logf("新版本校验通过")

	dir := execDir()
	exe := filepath.Join(dir, "V2bX")
	if err := copyFile(exe, exe+".bak", 0755); err != nil {
		return false, "", fmt.Errorf("备份旧版本失败: %w", err)
	}
	if err := copyFile(newBin, exe+".new", 0755); err != nil {
		return false, "", fmt.Errorf("写入新版本失败: %w", err)
	}
	if err := os.Rename(exe+".new", exe); err != nil {
		_ = os.Remove(exe + ".new")
		return false, "", fmt.Errorf("替换二进制失败: %w", err)
	}
	logf("已替换 %s（旧版本备份为 V2bX.bak）", exe)

	for _, f := range []string{"geoip.dat", "geosite.dat"} {
		src := filepath.Join(extractDir, f)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		_ = copyFile(src, filepath.Join(dir, f), 0644)
		_ = copyFile(src, filepath.Join("/etc/V2bX", f), 0644)
	}
	_ = os.WriteFile(filepath.Join(dir, ".version"), []byte(p.Version+"\n"), 0644)
	_ = os.WriteFile(versionFile, []byte(p.Version+"\n"), 0644)
	return true, ">>> 升级到 " + p.Version + " 完成，V2bX 即将重启", nil
}

func download(ctx context.Context, url, dest string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n == 0 {
		err = fmt.Errorf("文件为空")
	}
	return err
}

func unzip(src, dest string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	root := filepath.Clean(dest) + string(os.PathSeparator)
	for _, f := range zr.File {
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(target, root) {
			return fmt.Errorf("非法路径: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := extractFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	w, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, rc); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}
