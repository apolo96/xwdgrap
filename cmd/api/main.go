package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apolo96/gitfresh"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run() error {
	/* Logger */
	println("Config Agent Logger")
	file, closer, err := gitfresh.NewLogFile(gitfresh.APP_AGENT_LOG_FILE)
	if err != nil {
		return err
	}
	defer closer()
	logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger = logger.With("version", "1.0.0")
	slog.SetDefault(logger)
	/* loading agent */
	slog.Info("Loading GitFresh Agent")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	/* tunnel to localserver  channel communication */
	ch := make(chan string)
	defer close(ch)
	/* Check tunnel and localserver status */
	var wg sync.WaitGroup
	wg.Add(2)
	errch := make(chan error, 2)
	done := make(chan struct{})
	/* Start internet tunnel */
	go func() {
		slog.Info("Start Internet Tunnel")
		userPath, err := os.UserHomeDir()
		if err != nil {
			slog.Error("error getting user home directory", "error", err.Error())
			cancel()
		}
		path := filepath.Join(userPath, gitfresh.APP_FOLDER)
		appConfig := gitfresh.NewAppConfigSvc(
			logger, &gitfresh.FlatFile{
				Name: gitfresh.APP_CONFIG_FILE_NAME,
				Path: path,
			},
		)
		if err := tunnel(context.Background(), ch, appConfig, &wg); err != nil {
			slog.Error("tunnel failed", "error", err.Error())
			errch <- err
		}
	}()
	/* Start localserver */
	go func() {
		slog.Info("Start Local Serve")
		if err := localserver(ch, &wg); err != nil {
			slog.Error("localserver failed", "error", err.Error())
			errch <- err
		}
	}()
	/* Waiting for tunnel and localserver */
	go func() {
		wg.Wait()
		done <- struct{}{}
	}()
	/* Waiting timeout or tunnel & localserver started successfully */
	select {
	case <-ctx.Done():
		slog.Error("context done", "error", ctx.Err().Error())
		return ctx.Err()
	case <-done:
		slog.Info("servers are ready")
	}
	/* Waiting for errors from  tunnel or localserver */
	return <-errch
}

func localserver(ch chan string, wg *sync.WaitGroup) error {
	url := <-ch
	slog.Info("startup Local Serve then tunnel started")
	server := &http.Server{
		Addr: gitfresh.API_AGENT_HOST,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data := fmt.Sprintf(`{"api_version":"1.0.0", "tunnel_domain":"%s"}`, url)
			w.Header().Set("Content-type", "application/json")
			w.Write([]byte(data))
		}),
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		slog.Error("listening server", "error", err.Error())
		return err
	}
	slog.Info("channel done comunication")
	wg.Done()
	println("LocalServer Listening on " + server.Addr)
	slog.Info("LocalServer Listening on " + server.Addr)
	return server.Serve(listener)
}

func tunnel(ctx context.Context,
	ch chan<- string,
	appConfig *gitfresh.AppConfigSvc,
	wg *sync.WaitGroup,
) error {
	conf, err := appConfig.ReadConfigFile()
	if err != nil {
		slog.Error(err.Error())
		return err
	}
	slog.Debug("load agent config from file", "config", fmt.Sprint(conf))
	os.Setenv("NGROK_AUTHTOKEN", conf.TunnelToken)
	listener, err := ngrok.Listen(ctx,
		config.HTTPEndpoint(
			config.WithWebhookVerification("github", conf.GitHookSecret),
			config.WithDomain("yak-loyal-violently.ngrok-free.app"),
		),
		ngrok.WithAuthtokenFromEnv(),
	)
	if err != nil {
		slog.Error("listening tunnel", "error", err.Error())
		return err
	}
	ch <- listener.URL()
	wg.Done()
	println("Tunnel Listening on " + listener.URL())
	slog.Info("Tunnel Listening on " + listener.URL())
	return http.Serve(listener, handler(appConfig))
}

func handler(appConfig *gitfresh.AppConfigSvc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-GitHub-Event") == "ping" {
			slog.Info("handling ping", "hook_id", r.Header.Get("X-GitHub-Hook-ID"))
			w.WriteHeader(http.StatusOK)
			return
		}
		slog.Info("handling webhook", "hook_id", r.Header.Get("X-GitHub-Hook-ID"))
		form, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error(err.Error())
			http.Error(w, "error reading request data", http.StatusBadRequest)
			return
		}
		var p gitfresh.APIPayload
		if err := json.Unmarshal([]byte(form), &p); err != nil {
			slog.Error(err.Error())
			http.Error(w, "error parsing data form", http.StatusBadRequest)
			return
		}
		slog.Info(
			"payload form data",
			"branch", p.Ref,
			"repository", p.Repository.Name,
			"last_commit", p.Commit[:7],
		)
		w.WriteHeader(http.StatusOK)
		go func() {
			conf, err := appConfig.ReadConfigFile()
			if err != nil {
				slog.Error(err.Error())
			}
			gitPullCmd(conf.GitWorkDir, p.Repository.Name, p.Ref)
		}()
	})
}

func gitPullCmd(workdir, repoName, branch string) (err error) {
	p, err := exec.LookPath("git")
	if err != nil {
		slog.Error("which git path", "error", err.Error())
		return err
	}
	path := strings.ReplaceAll(string(p), "\n", "")
	slog.Info("which command output", "path", path)
	workspace := filepath.Join(workdir, repoName)
	cmd := exec.Command(path, "pull", "origin", branch)
	cmd.Dir = workspace
	slog.Info("running command ", "cmd", cmd.String(), "dir", cmd.Dir)
	err = cmd.Run()
	if err != nil {
		slog.Error("git command failed", "error", err.Error())
	}
	return err
}
