package prometheus

/*	License: GPLv3
	Authors:
		Mirko Brombin <mirko@fabricators.ltd>
		Vanilla OS Contributors <https://github.com/vanilla-os/>
	Copyright: 2023
	Description:
		Prometheus is a simple and accessible library for pulling and mounting
		container images. It is designed to be used as a dependency in ABRoot
		and Albius.
*/

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	cstorage "github.com/containers/storage"
)

/* NewPrometheus creates a new Prometheus instance, note that currently
 * Prometheus only works with custom stores, so you need to pass the
 * runRoot, graphRoot and graphDriverName to create a new one.
 */
func NewPrometheus(runRoot, graphRoot, graphDriverName string) (*Prometheus, error) {
	var err error

	runRoot = filepath.Clean(runRoot)
	if _, err := os.Stat(runRoot); os.IsNotExist(err) {
		return nil, errors.New("defined runRoot path does not exist: " + runRoot)
	}

	graphRoot = filepath.Clean(graphRoot)
	if _, err := os.Stat(graphRoot); os.IsNotExist(err) {
		return nil, errors.New("defined graphRoot path does not exist: " + graphRoot)
	}

	store, err := cstorage.GetStore(cstorage.StoreOptions{
		RunRoot:         runRoot,
		GraphRoot:       graphRoot,
		GraphDriverName: graphDriverName,
	})
	if err != nil {
		return nil, err
	}

	return &Prometheus{Store: store}, nil
}

/* PullImage pulls an image from a remote registry and stores it in the
 * Prometheus store. It returns the manifest of the pulled image and an
 * error if any. Note that the 'docker://' prefix is automatically added
 * to the imageName to make it compatible with the alltransports.ParseImageName
 * method. */
func (p *Prometheus) PullImage(imageName string, dstName string) (*OciManifest, error) {
	srcRef, err := alltransports.ParseImageName(fmt.Sprintf("docker://%s", imageName))
	if err != nil {
		return nil, err
	}

	destRef, err := storage.Transport.ParseStoreReference(p.Store, dstName)
	if err != nil {
		return nil, err
	}

	systemCtx := &types.SystemContext{}
	policy, err := signature.DefaultPolicy(systemCtx)
	if err != nil {
		return nil, err
	}

	policyCtx, err := signature.NewPolicyContext(policy)
	if err != nil {
		return nil, err
	}

	pulledManifestBytes, err := copy.Image(
		context.Background(),
		policyCtx,
		destRef,
		srcRef,
		&copy.Options{
			ReportWriter: os.Stdout,
		},
	)
	if err != nil {
		return nil, err
	}

	var manifest OciManifest
	err = json.Unmarshal(pulledManifestBytes, &manifest)
	if err != nil {
		return nil, err
	}

	// here we remove the 'sha256:' prefix from the digest, so we don't have
	// to deal with it later
	manifest.Config.Digest = manifest.Config.Digest[7:]
	for i := range manifest.Layers {
		manifest.Layers[i].Digest = manifest.Layers[i].Digest[7:]
	}

	return &manifest, nil
}

/* GetImageByDigest returns an image from the Prometheus store by its digest. */
func (p *Prometheus) GetImageByDigest(digest string) (cstorage.Image, error) {
	images, err := p.Store.Images()
	if err != nil {
		return cstorage.Image{}, err
	}

	for _, img := range images {
		if img.ID == digest {
			return img, nil
		}
	}

	err = cstorage.ErrImageUnknown
	return cstorage.Image{}, err
}

/* DoesImageExist checks if an image exists in the Prometheus store by its
 * digest. It returns a boolean indicating if the image exists and an error
 * if any. */
func (p *Prometheus) DoesImageExist(digest string) (bool, error) {
	image, err := p.GetImageByDigest(digest)
	if err != nil {
		return false, err
	}

	if image.ID == digest {
		return true, nil
	}

	return false, nil
}

/* MountImage mounts an image from the Prometheus store by its main layer
 * digest. It returns the mount path and an error if any. */
func (p *Prometheus) MountImage(layerId string) (string, error) {
	mountPath, err := p.Store.Mount(layerId, "")
	if err != nil {
		return "", err
	}

	return mountPath, nil
}

/* UnMountImage unmounts an image from the Prometheus store by its main layer
 * digest. It returns a boolean indicating if the unmount was successful and
 * an error if any. */
func (p *Prometheus) UnMountImage(layerId string, force bool) (bool, error) {
	res, err := p.Store.Unmount(layerId, force)
	if err != nil {
		return res, err
	}

	return res, nil
}
