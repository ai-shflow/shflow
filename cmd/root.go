package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	"github.com/ai-flowx/flowx/agent"
	"github.com/ai-flowx/flowx/config"
	"github.com/ai-flowx/flowx/flow"
	"github.com/ai-flowx/flowx/gpt"
	"github.com/ai-flowx/flowx/memory"
	"github.com/ai-flowx/flowx/prompt"
	"github.com/ai-flowx/flowx/store"
	"github.com/ai-flowx/flowx/tool"
)

const (
	routineNum = -1
)

var (
	configFile string
	listenPort string
)

var rootCmd = &cobra.Command{
	Use:     "flowx",
	Version: config.Version + "-build-" + config.Build,
	Short:   "ai framework",
	Long:    fmt.Sprintf("ai framework %s (%s %s)", config.Version, config.Commit, config.Build),
	Run: func(cmd *cobra.Command, args []string) {
		var cfg config.Config
		ctx := context.Background()
		if err := viper.Unmarshal(&cfg); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		g, err := initGpt(ctx, &cfg)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		p, err := initPrompt(ctx, &cfg)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		s, err := initStore(ctx, &cfg)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		m, err := initMemory(ctx, &cfg, s)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		t, err := initTool(ctx, &cfg, g)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		a, err := initAgent(ctx, &cfg, g, p, t)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		f, err := initFlow(ctx, &cfg, g, m, t, a)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		if err := runFlow(ctx, f); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// nolint:gochecknoinits
func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&configFile, "config-file", "c", "", "config file")
	rootCmd.PersistentFlags().StringVarP(&listenPort, "listen-port", "l", ":8080", "listen port")

	rootCmd.Root().CompletionOptions.DisableDefaultCmd = true
}

func initConfig() {
	if configFile == "" {
		return
	}

	viper.SetConfigFile(configFile)
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
	}
}

func initGpt(ctx context.Context, cfg *config.Config) (gpt.Gpt, error) {
	c := gpt.DefaultConfig()
	if c == nil {
		return nil, errors.New("failed to config\n")
	}

	c.Provider = cfg.Gpt.Provider
	c.Api = cfg.Gpt.Api
	c.Key = cfg.Gpt.Key
	c.Endpoint = cfg.Gpt.Endpoint

	return gpt.New(ctx, c), nil
}

func initPrompt(ctx context.Context, cfg *config.Config) (prompt.Prompt, error) {
	c := prompt.DefaultConfig()
	if c == nil {
		return nil, errors.New("failed to config\n")
	}

	c.Provider = cfg.Prompt.Provider
	c.Host = cfg.Prompt.Host
	c.Port = cfg.Prompt.Port
	c.User = cfg.Prompt.User
	c.Pass = cfg.Prompt.Pass

	return prompt.New(ctx, c), nil
}

func initStore(ctx context.Context, cfg *config.Config) (store.Store, error) {
	c := store.DefaultConfig()
	if c == nil {
		return nil, errors.New("failed to config\n")
	}

	c.Provider = cfg.Store.Provider
	c.Host = cfg.Store.Host
	c.Port = cfg.Store.Port
	c.Path = cfg.Store.Path
	c.User = cfg.Store.User
	c.Pass = cfg.Store.Pass

	return store.New(ctx, c), nil
}

func initMemory(ctx context.Context, cfg *config.Config, st store.Store) (memory.Memory, error) {
	c := memory.DefaultConfig()
	if c == nil {
		return nil, errors.New("failed to config\n")
	}

	c.Store = st
	c.Type = cfg.Memory.Type

	return memory.New(ctx, c), nil
}

func initTool(ctx context.Context, cfg *config.Config, _gpt gpt.Gpt) (tool.Tool, error) {
	c := tool.DefaultConfig()
	if c == nil {
		return nil, errors.New("failed to config\n")
	}

	c.Gpt = _gpt

	for _, item := range cfg.Tool {
		c.Provider = append(c.Provider, tool.Provider{
			Name: item.Name,
		})
	}

	return tool.New(ctx, c), nil
}

func initAgent(ctx context.Context, _ *config.Config, _gpt gpt.Gpt, _prompt prompt.Prompt, _tool tool.Tool) (agent.Agent, error) {
	c := agent.DefaultConfig()
	if c == nil {
		return nil, errors.New("failed to config\n")
	}

	c.Gpt = _gpt
	c.Prompt = _prompt
	c.Tool = _tool

	return agent.New(ctx, c), nil
}

func initFlow(ctx context.Context, cfg *config.Config, _gpt gpt.Gpt, mem memory.Memory,
	_tool tool.Tool, _agent agent.Agent) (flow.Flow, error) {
	c := flow.DefaultConfig()
	if c == nil {
		return nil, errors.New("failed to config\n")
	}

	c.Channel = cfg.Flow.Channel
	c.Port = listenPort
	c.Gpt = _gpt
	c.Memory = mem
	c.Tool = _tool
	c.Agent = _agent

	return flow.New(ctx, c), nil
}

func runFlow(ctx context.Context, fl flow.Flow) error {
	if err := fl.Init(ctx); err != nil {
		return errors.Wrap(err, "failed to init\n")
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(routineNum)

	g.Go(func() error {
		if err := fl.Run(ctx); err != nil {
			return errors.Wrap(err, "failed to run\n")
		}
		return nil
	})

	s := make(chan os.Signal, 1)

	// kill (no param) default send syscanll.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can"t be caught, so don't need add it
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)

	g.Go(func() error {
		<-s
		if err := fl.Deinit(ctx); err != nil {
			return errors.Wrap(err, "failed to deinit\n")
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}
