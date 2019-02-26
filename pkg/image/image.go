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

package image

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// SaveToTar takes a list of images and writes them to a tarbal
func SaveToTar(ctx context.Context, cli *client.Client, images []string, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return errors.Wrapf(err, "Could not create tarball file '%s'", filepath)
	}
	defer file.Close()

	out, err := cli.ImageSave(ctx, images)
	if err != nil {
		return errors.Wrap(err, "error saving images to tar")
	}

	_, err = io.Copy(file, out)
	if err != nil {
		return errors.Wrapf(err, "Could not copy the file '%s' data to the tarball", filepath)
	}

	// Wait for all data to complete
	_, err = ioutil.ReadAll(out)
	if err != nil {
		return errors.Wrap(err, "error exporting images")
	}

	return nil
}

// PullImage pulls an image from a registry to the local docker client
func PullImage(ctx context.Context, cli *client.Client, img Config) error {
	out, err := cli.ImagePull(ctx, img.GetE2EImage(), types.ImagePullOptions{})
	if err != nil {
		return errors.Wrapf(err, "error pulling image: %v", img.GetE2EImage())
	}
	defer out.Close()

	// Show status
	err = streamDockerMessages(out)
	if err != nil {
		return errors.Wrapf(err, "error pulling image: %v", img.GetE2EImage())
	}

	// Wait for all data to complete
	_, err = ioutil.ReadAll(out)
	if err != nil {
		return errors.Wrapf(err, "error pulling image: %v", img.GetE2EImage())
	}

	return nil
}

// DeleteImage deletes an image from the local docker client
func DeleteImage(ctx context.Context, cli *client.Client, img Config) ([]types.ImageDeleteResponseItem, error) {
	out, err := cli.ImageRemove(ctx, img.GetE2EImage(), types.ImageRemoveOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error deleting image: %v", img.GetE2EImage())
	}

	return out, nil
}

// GetImages gets a map of image Configs
func GetImages(e2eRegistryConfig, version string) (map[string]Config, error) {
	// Get list of upstream images that match the version
	reg, err := NewRegistryList(e2eRegistryConfig, version)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't init Registry List")
	}

	imgs, err := reg.GetImageConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "couldn't get images for version")
	}
	return imgs, nil
}

// TagImage tags an image in the local docker client
func TagImage(ctx context.Context, cli *client.Client, srcimg Config, destimg Config) error {
	fmt.Printf("Tagging image: %v to %v\n", srcimg.GetE2EImage(), destimg.GetE2EImage())
	err := cli.ImageTag(ctx, srcimg.GetE2EImage(), destimg.GetE2EImage())
	if err != nil {
		return errors.Wrapf(err, "error tagging image: %v", destimg.GetE2EImage())
	}
	return nil
}

// PushImage pushed an image to a docker registry
func PushImage(ctx context.Context, cli *client.Client, img Config, auth types.AuthConfig) error {
	authBytes, err := json.Marshal(auth)
	if err != nil {
		return errors.Wrap(err, "error marshaling username/password")
	}

	authBase64 := base64.URLEncoding.EncodeToString(authBytes)

	out, err := cli.ImagePush(ctx, img.GetE2EImage(), types.ImagePushOptions{
		RegistryAuth: authBase64,
	})
	if err != nil {
		return errors.Wrapf(err, "error pushing image: %v", img.GetE2EImage())
	}
	defer out.Close()

	// Show status
	err = streamDockerMessages(out)
	if err != nil {
		return errors.Wrapf(err, "error uploading image: %v", img.GetE2EImage())
	}

	_, err = ioutil.ReadAll(out)
	if err != nil {
		return errors.Wrapf(err, "error uploading image: %v", img.GetE2EImage())
	}

	return nil
}

// GetTarFileName returns a filename matching the version of Kubernetes images are exported
func GetTarFileName(version string) string {
	return fmt.Sprintf("kubernetes_e2e_images_%s.tar", version)
}

func streamDockerMessages(src io.Reader) error {
	fd, _ := term.GetFdInfo(os.Stderr)
	return jsonmessage.DisplayJSONMessagesStream(src, os.Stderr, fd, true, nil)
}
