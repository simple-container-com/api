package kubernetes

import (
	"embed"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// AddVolumeMounts adds volume mounts for a given volume name and list of volumes
func AddVolumeMounts(vName string, volumes []k8s.SimpleTextVolume, mounts *[]corev1.VolumeMountArgs) {
	for _, v := range volumes {
		*mounts = append(*mounts, corev1.VolumeMountArgs{
			Name:      pulumi.String(vName),
			MountPath: pulumi.String(v.MountPath),
			SubPath:   pulumi.String(v.Name),
		})
	}
}

// DirToConfigMapVolumes converts a directory's files to SimpleTextVolume objects
func DirToConfigMapVolumes(dirPath string, mountPathBase string) ([]k8s.SimpleTextVolume, error) {
	var res []k8s.SimpleTextVolume
	err := addFilesFromDir(dirPath, &res, func(fullFilePath string) {
		subPath, err := filepath.Rel(dirPath, fullFilePath)
		if err != nil {
			return
		}
		cfgKey := filepath.ToSlash(subPath)
		volume := k8s.SimpleTextVolume{
			TextVolume: api.TextVolume{
				Content:   readFile(fullFilePath),
				Name:      cfgKey,
				MountPath: filepath.Join(mountPathBase, subPath),
			},
		}
		res = append(res, volume)
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// addFilesFromDir recursively adds files from the given directory to a result slice of SimpleTextVolume
func addFilesFromDir(dirPath string, res *[]k8s.SimpleTextVolume, proc func(fullFilePath string)) error {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}

	for _, file := range files {
		fullFilePath := filepath.Join(dirPath, file.Name())
		if file.IsDir() {
			err := addFilesFromDir(fullFilePath, res, proc)
			if err != nil {
				return err
			}
		} else {
			proc(fullFilePath)
		}
	}
	return nil
}

func EmbedFSToTextVolumes(volumes []k8s.SimpleTextVolume, fs embed.FS, dir string, baseDir string) ([]k8s.SimpleTextVolume, error) {
	files, err := fs.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read dir %q", dir)
	}
	for _, file := range files {
		if file.IsDir() {
			subdir := path.Join(dir, file.Name())
			if volumes, err = EmbedFSToTextVolumes(volumes, fs, subdir, filepath.Join(baseDir, file.Name())); err != nil {
				return nil, errors.Wrapf(err, "failed to read subdir %q", subdir)
			}
			continue
		}
		if content, err := fs.ReadFile(path.Join(dir, file.Name())); err != nil {
			return nil, errors.Wrapf(err, "failed to read file %q", file.Name())
		} else {
			volumes = append(volumes, k8s.SimpleTextVolume{
				TextVolume: api.TextVolume{
					Content:   string(content),
					Name:      file.Name(),
					MountPath: filepath.Join(baseDir, file.Name()),
				},
			})
		}
	}
	return volumes, nil
}

// Helper function to read the file contents as a string
func readFile(fullFilePath string) string {
	content, err := os.ReadFile(fullFilePath)
	if err != nil {
		return ""
	}
	return string(content)
}
