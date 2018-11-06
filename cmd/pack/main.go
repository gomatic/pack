package main

import (
	"fmt"
	"github.com/buildpack/pack/style"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
)

var Version = "UNKNOWN"

func main() {
	rootCmd := &cobra.Command{Use: "pack"}
	for _, f := range [](func() *cobra.Command){
		buildCommand,
		runCommand,
		rebaseCommand,
		createBuilderCommand,
		addStackCommand,
		updateStackCommand,
		deleteStackCommand,
		setDefaultStackCommand,
		versionCommand,
	} {
		rootCmd.AddCommand(f())
	}
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildCommand() *cobra.Command {
	var buildFlags pack.BuildFlags
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("build %s", style.Help("<image-name>")),
		Args:  cobra.ExactArgs(1),
		Short: style.Help("Generate app image from source code"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			buildFlags.RepoName = args[0]
			bf, err := pack.DefaultBuildFactory()
			if err != nil {
				return err
			}
			b, err := bf.BuildConfigFromFlags(&buildFlags)
			if err != nil {
				return err
			}
			return b.Run()
		},
	}
	cmd.Flags().StringVarP(&buildFlags.AppDir, "path", "p", "current working directory", style.Help("Path to app dir"))
	cmd.Flags().StringVar(&buildFlags.Builder, "builder", "packs/samples", style.Help("Builder"))
	cmd.Flags().StringVar(&buildFlags.RunImage, "run-image", "default stack run image", style.Help("Run image"))
	cmd.Flags().BoolVar(&buildFlags.Publish, "publish", false, style.Help("Publish to registry"))
	cmd.Flags().BoolVar(&buildFlags.NoPull, "no-pull", false, style.Help("Skip pulling images before use"))
	cmd.Flags().StringArrayVar(&buildFlags.Buildpacks, "buildpack", []string{}, style.Help("Buildpack ID or host directory path, \nrepeat for each buildpack in order"))
	cmd.Flags().BoolP("help", "h", false, style.Help("Help for build"))
	return cmd
}

func runCommand() *cobra.Command {
	wd, _ := os.Getwd()

	var runFlags pack.RunFlags
	cmd := &cobra.Command{
		Use:   "run",
		Args:  cobra.NoArgs,
		Short: style.Help("Build and run app image (recommended for development only)"),
		RunE: func(cmd *cobra.Command, args []string) error {
			bf, err := pack.DefaultBuildFactory()
			if err != nil {
				return err
			}
			r, err := bf.RunConfigFromFlags(&runFlags)
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true
			return r.Run(makeStopChannelForSignals)
		},
	}
	cmd.Flags().StringVarP(&runFlags.AppDir, "path", "p", wd, style.Help("Path to app directory"))
	cmd.Flags().StringVar(&runFlags.Builder, "builder", "packs/samples", style.Help("Builder"))
	cmd.Flags().StringVar(&runFlags.RunImage, "run-image", "default stack run image", style.Help("Run image"))
	cmd.Flags().StringVar(&runFlags.Port, "port", "ports exposed by container", style.Help("Comma separated ports to publish"))
	cmd.Flags().BoolP("help", "h", false, style.Help("Help for run"))
	return cmd
}

func rebaseCommand() *cobra.Command {
	var flags pack.RebaseFlags
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("rebase %s", style.Help("<image-name>")),
		Args:  cobra.ExactArgs(1),
		Short: style.Help("Rebase app image with latest run image"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			flags.RepoName = args[0]

			imageFactory, err := image.DefaultFactory()
			if err != nil {
				return err
			}
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			factory := pack.RebaseFactory{
				Log:          log.New(os.Stdout, "", log.LstdFlags),
				Config:       cfg,
				ImageFactory: imageFactory,
			}
			rebaseConfig, err := factory.RebaseConfigFromFlags(flags)
			if err != nil {
				return err
			}
			return factory.Rebase(rebaseConfig)
		},
	}
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, style.Help("Publish to registry"))
	cmd.Flags().BoolVar(&flags.NoPull, "no-pull", false, style.Help("Skip pulling images before use"))
	cmd.Flags().BoolP("help", "h", false, style.Help("Help for rebase"))
	return cmd
}

func createBuilderCommand() *cobra.Command {
	flags := pack.CreateBuilderFlags{}
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("create-builder %s -b %s", style.Help("<image-name>"), style.Help("<builder-toml-path>")),
		Args:  cobra.ExactArgs(1),
		Short: style.Help("Create builder image"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			flags.RepoName = args[0]

			docker, err := docker.New()
			if err != nil {
				return err
			}
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			builderFactory := pack.BuilderFactory{
				FS:     &fs.FS{},
				Log:    log.New(os.Stdout, "", log.LstdFlags),
				Docker: docker,
				Config: cfg,
				Images: &image.Client{},
			}
			builderConfig, err := builderFactory.BuilderConfigFromFlags(flags)
			if err != nil {
				return err
			}
			return builderFactory.Create(builderConfig)
		},
	}
	cmd.Flags().BoolVar(&flags.NoPull, "no-pull", false, style.Help("Skip pulling stack image before use"))
	cmd.Flags().StringVarP(&flags.BuilderTomlPath, "builder-config", "b", "", style.Help("Path to builder TOML file"))
	cmd.Flags().StringVarP(&flags.StackID, "stack", "s", "defined by set-default-stack command", style.Help("Stack name"))
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, style.Help("Publish to registry"))
	cmd.Flags().BoolP("help", "h", false, style.Help("Help for create-builder"))
	return cmd
}

func addStackCommand() *cobra.Command {
	flags := struct {
		BuildImages []string
		RunImages   []string
	}{}
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("add-stack %s --build-image %s --run-image %s", style.Help("<stack-name>"), style.Help("<build-image-name>"), style.Help("<run-image-name>")),
		Args:  cobra.ExactArgs(1),
		Short: style.Help("Add stack to list of available stacks"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			if err := cfg.Add(config.Stack{
				ID:          args[0],
				BuildImages: flags.BuildImages,
				RunImages:   flags.RunImages,
			}); err != nil {
				return err
			}
			fmt.Printf("%s successfully added\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&flags.BuildImages, "build-image", "b", []string{}, style.Help("Build image to associate with stack"))
	cmd.Flags().StringSliceVarP(&flags.RunImages, "run-image", "r", []string{}, style.Help("Run image to associate with stack,\nrepeat for each run image"))
	cmd.Flags().BoolP("help", "h", false, style.Help("Help for add-stack"))
	return cmd
}

func updateStackCommand() *cobra.Command {
	flags := struct {
		BuildImages []string
		RunImages   []string
	}{}
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("update-stack %s --build-image %s --run-image %s", style.Help("<stack-name>"), style.Help("<build-image-name>"), style.Help("<run-image-name>")),
		Args:  cobra.ExactArgs(1),
		Short: style.Help("Update stack build and run images"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := config.New(filepath.Join(os.Getenv("HOME"), ".pack"))
			if err != nil {
				return err
			}
			if err := cfg.Update(args[0], config.Stack{
				BuildImages: flags.BuildImages,
				RunImages:   flags.RunImages,
			}); err != nil {
				return err
			}
			fmt.Printf("%s successfully updated\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&flags.BuildImages, "build-image", "b", []string{}, style.Help("Build image to associate with stack"))
	cmd.Flags().StringSliceVarP(&flags.RunImages, "run-image", "r", []string{}, style.Help("Run image to associate with stack,\nrepeat for each run image"))
	cmd.Flags().BoolP("help", "h", false, style.Help("Help for update-stack"))
	return cmd
}

func deleteStackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("delete-stack %s", style.Help("<stack-name>")),
		Args:  cobra.ExactArgs(1),
		Short: style.Help("Delete stack from list of available stacks"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := config.New(filepath.Join(os.Getenv("HOME"), ".pack"))
			if err != nil {
				return err
			}
			if err := cfg.Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("%s has been successfully deleted\n", args[0])
			return nil
		},
	}
	return cmd
}

func setDefaultStackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("set-default-stack %s", style.Help("<stack-name>")),
		Args:  cobra.ExactArgs(1),
		Short: style.Help("Set default stack used by other commands"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := config.New(filepath.Join(os.Getenv("HOME"), ".pack"))
			if err != nil {
				return err
			}
			err = cfg.SetDefaultStack(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("%s is now the default stack\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolP("help", "h", false, style.Help("Help for set-default-stack"))
	return cmd
}

func versionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Args:  cobra.NoArgs,
		Short: style.Help("Show current pack version"),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(strings.TrimSpace(Version))
		},
	}
	cmd.Flags().BoolP("help", "h", false, style.Help("Help for version"))
	return cmd
}

func makeStopChannelForSignals() <-chan struct{} {
	sigsCh := make(chan os.Signal, 1)
	stopCh := make(chan struct{}, 1)
	signal.Notify(sigsCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		// convert chan os.Signal to chan struct{}
		for {
			<-sigsCh
			stopCh <- struct{}{}
		}
	}()
	return stopCh
}
