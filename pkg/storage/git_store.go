package storage

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

type GitStorage struct {
	repo *git.Repository

	sync.Mutex
}

func NewGitStorage(path string) (*GitStorage, error) {
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
	return &GitStorage{
		repo: repo,
	}, nil
}

func (s *GitStorage) addAndCommit(name string) (string, error) {
	worktree, err := s.repo.Worktree()
	if err != nil {
		return "", err
	}
	if _, err := worktree.Add(name); err != nil {
		return "", err
	}
	hash, err := worktree.Commit("test commit", &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  "Operator",
			Email: "operator@openshift.io",
			When:  time.Now(),
		},
		// TODO: Provide the CRD operator?
		Committer: &object.Signature{
			Name:  "Operator",
			Email: "operator@openshift.io",
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", err
	}
	return hash.String(), err
}

func (s *GitStorage) deleteFile(name string) error {
	worktree, err := s.repo.Worktree()
	if err != nil {
		return err
	}
	return worktree.Filesystem.Remove(name)
}

func (s *GitStorage) updateFile(name string, content []byte) error {
	worktree, err := s.repo.Worktree()
	if err != nil {
		return err
	}

	// Simple "create file"
	if _, err := worktree.Filesystem.Lstat(name); err != nil {
		if os.IsNotExist(err) {
			f, err := worktree.Filesystem.Create(name)
			if err != nil {
				return err
			}
			if _, err := f.Write(content); err != nil {
				return err
			}
			return f.Close()
		}
	}

	// Updating existing file by replacing the original
	if err := worktree.Filesystem.Remove(name); err != nil {
		return err
	}
	return s.updateFile(name, content)
}

func getConfigName(gvk schema.GroupVersionKind) string {
	return strings.ToLower(fmt.Sprintf("%s.%s.%s.json", gvk.Kind, gvk.Version, gvk.Group))
}

func (s *GitStorage) addConfigObject(obj interface{}) {
	s.Lock()
	defer s.Unlock()
	unstruct := obj.(*unstructured.Unstructured)
	objBytes, err := runtime.Encode(unstructured.UnstructuredJSONScheme, unstruct)
	if err != nil {
		panic(err)
	}
	klog.Infof("Observed change in %q ...", getConfigName(unstruct.GroupVersionKind()))
	if err := s.updateFile(getConfigName(unstruct.GroupVersionKind()), objBytes); err != nil {
		klog.Warningf("Failed to adding change in %q to GIT: %v", getConfigName(unstruct.GroupVersionKind()), err)
	}
	hash, err := s.addAndCommit(getConfigName(unstruct.GroupVersionKind()))
	if err != nil {
		klog.Warningf("Failed to commit change in %q to GIT: %v", getConfigName(unstruct.GroupVersionKind()), err)
	}

	klog.Infof("Committed %q change for %q", hash, getConfigName(unstruct.GroupVersionKind()))
}

func (s *GitStorage) updateConfigObject(old, new interface{}) {
	s.Lock()
	defer s.Unlock()
	s.addConfigObject(new)
}

func (s *GitStorage) deleteConfigObject(obj interface{}) {
	s.Lock()
	defer s.Unlock()
	// TODO: Deal with tombstone
	unstruct := obj.(*unstructured.Unstructured)
	if err := s.deleteFile(getConfigName(unstruct.GroupVersionKind())); err != nil {
		klog.Warningf("Failed to delete file: %q: %v", getConfigName(unstruct.GroupVersionKind()), err)
	}
	hash, err := s.addAndCommit(getConfigName(unstruct.GroupVersionKind()))
	if err != nil {
		klog.Warningf("Failed to commit change in %q to GIT: %v", getConfigName(unstruct.GroupVersionKind()), err)
	}

	klog.Infof("Committed %q change for %q", hash, getConfigName(unstruct.GroupVersionKind()))
}

func (s *GitStorage) EventHandlers() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    s.addConfigObject,
		UpdateFunc: s.updateConfigObject,
		DeleteFunc: s.deleteConfigObject,
	}
}
