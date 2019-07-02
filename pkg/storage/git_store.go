package storage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/yaml"
)

type GitStorage struct {
	repo *git.Repository
	path string

	// The storage must be synchronized.
	sync.Mutex
}

func NewGitStorage(path string) (cache.ResourceEventHandler, error) {
	// If the repo does not exists, do git init
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_, err := git.PlainInit(path, false)
		if err != nil {
			return nil, err
		}
	}
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}
	storage := &GitStorage{path: path, repo: repo}
	return storage, nil
}

func (s *GitStorage) OnAdd(obj interface{}) {
	s.Lock()
	defer s.Unlock()
	name, content, err := decodeUnstructuredObject(obj)
	if err != nil {
		klog.Warningf("Unable to decode %q: %v", name, err)
		return
	}
	if err := s.writeToFilesystem(name, content); err != nil {
		klog.Warningf("Unable to write file %q: %v", name, err)
		return
	}
	hash, err := s.commitFile(name, "operator", fmt.Sprintf("%q added", name))
	if err != nil {
		klog.Warningf("Unable to commit file %q: %v", name, err)
	}
	s.updateRefsFile()
	klog.Infof("Added %q in commit %q", name, hash)
}

func (s *GitStorage) OnUpdate(_, obj interface{}) {
	s.Lock()
	defer s.Unlock()
	name, content, err := decodeUnstructuredObject(obj)
	if err != nil {
		klog.Warningf("Unable to decode %q: %v", name, err)
		return
	}
	if err := s.writeToFilesystem(name, content); err != nil {
		klog.Warningf("Unable to write file %q: %v", name, err)
		return
	}
	hash, err := s.commitFile(name, "operator", fmt.Sprintf("%q updated", name))
	if err != nil {
		klog.Warningf("Unable to commit file %q: %v", name, err)
	}
	s.updateRefsFile()
	klog.Infof("Updated %q in commit %q", name, hash)
}

func (s *GitStorage) OnDelete(obj interface{}) {
	s.Lock()
	defer s.Unlock()
	name, _, err := decodeUnstructuredObject(obj)
	if err != nil {
		klog.Warningf("Unable to decode %q: %v", name, err)
		return
	}
	if err := s.deleteFile(name); err != nil {
		klog.Warningf("Unable to delete file %q: %v", name, err)
		return
	}
	hash, err := s.commitFile(name, "operator", fmt.Sprintf("%q removes", name))
	if err != nil {
		klog.Warningf("Unable to commit file %q: %v", name, err)
	}
	s.updateRefsFile()
	klog.Infof("Deleted %q in commit %q", name, hash)
}

func decodeUnstructuredObject(obj interface{}) (string, []byte, error) {
	objUnstructured := obj.(*unstructured.Unstructured)
	filename := getObjectFilename(objUnstructured.GroupVersionKind())
	objectBytes, err := runtime.Encode(unstructured.UnstructuredJSONScheme, objUnstructured)
	if err != nil {
		return filename, nil, err
	}
	objectYAML, err := yaml.JSONToYAML(objectBytes)
	if err != nil {
		return filename, nil, err
	}
	return filename, objectYAML, err
}

func getObjectFilename(gvk schema.GroupVersionKind) string {
	return strings.ToLower(fmt.Sprintf("%s.%s.%s.yaml", gvk.Kind, gvk.Version, gvk.Group))
}

func (s *GitStorage) commitFile(name, component, message string) (string, error) {
	t, err := s.repo.Worktree()
	if err != nil {
		return "", err
	}
	if _, err := t.Add(name); err != nil {
		return "", err
	}
	hash, err := t.Commit(message, &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  "config-history-operator",
			Email: "config-history-operator@openshift.io",
			When:  time.Now(),
		},
		Committer: &object.Signature{
			Name:  component,
			Email: component + "@openshift.io",
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", err
	}
	return hash.String(), err
}

func (s *GitStorage) deleteFile(name string) error {
	t, err := s.repo.Worktree()
	if err != nil {
		return err
	}
	return t.Filesystem.Remove(name)
}

func (s *GitStorage) writeToFilesystem(name string, content []byte) error {
	t, err := s.repo.Worktree()
	if err != nil {
		return err
	}

	if _, err := t.Filesystem.Lstat(name); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		f, err := t.Filesystem.Create(name)
		if err != nil {
			return err
		}
		if _, err := f.Write(content); err != nil {
			return err
		}
		return f.Close()
	}

	if err := s.deleteFile(name); err != nil {
		return err
	}

	return s.writeToFilesystem(name, content)
}

// updateRefsFile populate .git/info/refs which is needed for git clone
func (s *GitStorage) updateRefsFile() {
	refs, _ := s.repo.References()
	var data []byte
	err := refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.HashReference {
			data = append(data, []byte(fmt.Sprintf("%s\n", ref))...)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Join(s.path, ".git", "info"), os.ModePerm); err != nil {
		panic(err)
	}
	if err := ioutil.WriteFile(filepath.Join(s.path, ".git", "info", "refs"), data, os.ModePerm); err != nil {
		panic(err)
	}
}
