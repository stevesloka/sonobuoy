/*
Copyright 2019 Heptio Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"fmt"
	"os"
	"syscall"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/heptio/sonobuoy/pkg/errlog"
	"github.com/heptio/sonobuoy/pkg/image"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/context"
)

var imagesflags imagesFlags

const dockerClientVersion = "1.37"

type imagesFlags struct {
	e2eRegistryConfig  string
	plugin             string
	username           string
	password           string
	imagesSaveFileName string
	downloadTar        bool
	kubeconfig         Kubeconfig
}

func NewCmdImages() *cobra.Command {
	// Main command
	cmd := &cobra.Command{
		Use:   "images",
		Short: "Manage images used in a plugin. Supported plugins are: 'e2e'",
		Run:   listImages,
		Args:  cobra.ExactArgs(0),
	}

	AddKubeconfigFlag(&imagesflags.kubeconfig, cmd.Flags())
	AddPluginFlag(&imagesflags.plugin, cmd.Flags())

	// Pull command
	pullCmd := &cobra.Command{
		Use:   "pull",
		Short: "Pulls images to local docker client for a specific plugin",
		Run:   pullImages,
		Args:  cobra.ExactArgs(0),
	}
	AddKubeconfigFlag(&imagesflags.kubeconfig, pullCmd.Flags())
	AddPluginFlag(&imagesflags.plugin, pullCmd.Flags())

	// Download command
	downloadCmd := &cobra.Command{
		Use:   "download",
		Short: "Saves downloaded images from local docker client to a tar file",
		Run:   downloadImages,
		Args:  cobra.ExactArgs(0),
	}
	AddKubeconfigFlag(&imagesflags.kubeconfig, downloadCmd.Flags())
	AddPluginFlag(&imagesflags.plugin, downloadCmd.Flags())

	// Push command
	pushCmd := &cobra.Command{
		Use:   "push",
		Short: "Pushes images to docker registry for a specific plugin",
		Run:   pushImages,
		Args:  cobra.ExactArgs(0),
	}
	AddE2ERegistryConfigFlag(&imagesflags.e2eRegistryConfig, pushCmd.Flags())
	AddKubeconfigFlag(&imagesflags.kubeconfig, pushCmd.Flags())
	AddPluginFlag(&imagesflags.plugin, pushCmd.Flags())
	AddRegistryUsernameFlag(&imagesflags.username, pushCmd.Flags())
	pushCmd.MarkFlagRequired(e2eRegistryConfigFlag)

	// Delete command
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes all images downloaded to local docker client",
		Run:   deleteImages,
		Args:  cobra.ExactArgs(0),
	}
	AddE2ERegistryConfigFlag(&imagesflags.e2eRegistryConfig, deleteCmd.Flags())
	AddKubeconfigFlag(&imagesflags.kubeconfig, deleteCmd.Flags())
	AddPluginFlag(&imagesflags.plugin, deleteCmd.Flags())

	viper.AutomaticEnv()

	cmd.AddCommand(pullCmd)
	cmd.AddCommand(pushCmd)
	cmd.AddCommand(downloadCmd)
	cmd.AddCommand(deleteCmd)

	return cmd
}

func listImages(cmd *cobra.Command, args []string) {

	if len(imagesflags.e2eRegistryConfig) > 0 {
		// Check if the e2e file exists
		if _, err := os.Stat(imagesflags.e2eRegistryConfig); os.IsNotExist(err) {
			errlog.LogError(errors.Errorf("file does not exist or cannot be opened: %v", imagesflags.e2eRegistryConfig))
			os.Exit(1)
		}
	}

	switch imagesflags.plugin {
	case "e2e":

		cfg, err := imagesflags.kubeconfig.Get()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get REST client"))
			os.Exit(1)
		}

		sbc, err := getSonobuoyClient(cfg)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "could not create sonobuoy client"))
			os.Exit(1)
		}

		version, err := sbc.Version()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get Sonobuoy client"))
			os.Exit(1)
		}

		// Get list of images that match the version
		registry, err := image.NewRegistryList("", version)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't init Registry List"))
			os.Exit(1)
		}

		images, err := registry.GetImageConfigs()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get images for version"))
			os.Exit(1)
		}

		for _, v := range images {
			fmt.Println(v.GetE2EImage())
		}
	default:
		errlog.LogError(errors.Errorf("Unsupported plugin: %v", imagesflags.plugin))
		os.Exit(1)
	}
}

func pullImages(cmd *cobra.Command, args []string) {
	switch imagesflags.plugin {
	case "e2e":

		cfg, err := imagesflags.kubeconfig.Get()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get REST client"))
			os.Exit(1)
		}

		sbc, err := getSonobuoyClient(cfg)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "could not create sonobuoy client"))
			os.Exit(1)
		}

		version, err := sbc.Version()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get Sonobuoy client"))
			os.Exit(1)
		}

		upstreamImages, err := image.GetImages("", version)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't init upstream registry list"))
			os.Exit(1)
		}

		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.WithVersion(dockerClientVersion))
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't init docker client"))
			os.Exit(1)
		}

		for _, v := range upstreamImages {
			err = image.PullImage(ctx, cli, v)
			if err != nil {
				errlog.LogError(errors.Wrapf(err, "couldn't pull image: %v", v.GetE2EImage()))
			}
			fmt.Println("########")
		}
	default:
		errlog.LogError(errors.Errorf("Unsupported plugin: %v", imagesflags.plugin))
		os.Exit(1)
	}
}

func downloadImages(cmd *cobra.Command, args []string) {
	switch imagesflags.plugin {
	case "e2e":

		cfg, err := imagesflags.kubeconfig.Get()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get REST client"))
			os.Exit(1)
		}

		sbc, err := getSonobuoyClient(cfg)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "could not create sonobuoy client"))
			os.Exit(1)
		}

		version, err := sbc.Version()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get Sonobuoy client"))
			os.Exit(1)
		}

		upstreamImages, err := image.GetImages("", version)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't init upstream registry list"))
			os.Exit(1)
		}

		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.WithVersion(dockerClientVersion))
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't init docker client"))
			os.Exit(1)
		}

		images := []string{}
		for _, v := range upstreamImages {
			images = append(images, v.GetE2EImage())
		}
		err = image.SaveToTar(ctx, cli, images, image.GetTarFileName(version))
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't save images to tar"))
			os.Exit(1)
		}

	default:
		errlog.LogError(errors.Errorf("Unsupported plugin: %v", imagesflags.plugin))
		os.Exit(1)
	}
}

func pushImages(cmd *cobra.Command, args []string) {

	switch imagesflags.plugin {
	case "e2e":

		// Check if the e2e file exists
		if _, err := os.Stat(imagesflags.e2eRegistryConfig); os.IsNotExist(err) {
			errlog.LogError(errors.Errorf("file does not exist or cannot be opened: %v", imagesflags.e2eRegistryConfig))
			os.Exit(1)
		}

		// Check if username if specified for registry auth
		if len(imagesflags.username) > 0 {
			// Check if password was set via ENV variable, otherwise prompt user for password from STDIN
			envPassword := viper.Get("password").(string)
			if len(envPassword) == 0 {
				fmt.Print("Registry password: ")
				bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
				if err != nil {
					errlog.LogError(errors.Wrap(err, "couldn't get password from user"))
					os.Exit(1)
				}

				imagesflags.password = string(bytePassword)
				fmt.Print("\n")
			} else {
				imagesflags.password = envPassword
			}
		}

		cfg, err := imagesflags.kubeconfig.Get()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get REST client"))
			os.Exit(1)
		}

		sbc, err := getSonobuoyClient(cfg)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "could not create sonobuoy client"))
			os.Exit(1)
		}

		version, err := sbc.Version()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get Sonobuoy client"))
			os.Exit(1)
		}

		upstreamImages, err := image.GetImages("", version)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't init upstream registry list"))
			os.Exit(1)
		}

		privateImages, err := image.GetImages(imagesflags.e2eRegistryConfig, version)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't init upstream registry list"))
			os.Exit(1)
		}

		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.WithVersion(dockerClientVersion))
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't init docker client"))
			os.Exit(1)
		}

		auth := types.AuthConfig{
			Username: imagesflags.username,
			Password: imagesflags.password,
		}

		for k, v := range upstreamImages {
			err = image.TagImage(ctx, cli, v, privateImages[k])
			if err != nil {
				errlog.LogError(errors.Wrapf(err, "couldn't tag image: %v", v.GetE2EImage()))
			}

			err = image.PushImage(ctx, cli, privateImages[k], auth)
			if err != nil {
				errlog.LogError(errors.Wrapf(err, "couldn't push image: %v", v.GetE2EImage()))
			}
			fmt.Println("########")
		}
	default:
		errlog.LogError(errors.Errorf("Unsupported plugin: %v", imagesflags.plugin))
		os.Exit(1)
	}

}

func deleteImages(cmd *cobra.Command, args []string) {
	switch imagesflags.plugin {
	case "e2e":

		cfg, err := imagesflags.kubeconfig.Get()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get REST client"))
			os.Exit(1)
		}

		sbc, err := getSonobuoyClient(cfg)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "could not create sonobuoy client"))
			os.Exit(1)
		}

		version, err := sbc.Version()
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't get Sonobuoy client"))
			os.Exit(1)
		}

		upstreamImages, err := image.GetImages(imagesflags.e2eRegistryConfig, version)
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't init upstream registry list"))
			os.Exit(1)
		}

		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.WithVersion(dockerClientVersion))
		if err != nil {
			errlog.LogError(errors.Wrap(err, "couldn't init docker client"))
			os.Exit(1)
		}

		for _, v := range upstreamImages {
			resp, err := image.DeleteImage(ctx, cli, v)
			if err != nil {
				errlog.LogError(errors.Wrapf(err, "couldn't delete image: %v", v.GetE2EImage()))
			}

			for _, r := range resp {
				fmt.Printf("Deleted: %v\n", r.Deleted)
				if len(r.Untagged) > 0 {
					fmt.Printf("Untagged: %v\n", r.Untagged)
				}
			}

			fmt.Println("########")
		}
	default:
		errlog.LogError(errors.Errorf("Unsupported plugin: %v", imagesflags.plugin))
		os.Exit(1)
	}
}
