/*
  Copyright 2026 Amazon.com, Inc. or its affiliates. All Rights Reserved.

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

package handler

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	storageinformers "k8s.io/client-go/informers/storage/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// CSIDriverChecker reports whether a CSIDriver object is installed in the
// cluster.
type CSIDriverChecker interface {
	// Present reports whether the named CSIDriver exists. known is false when
	// the answer is not available, for example before the informer cache has
	// synced or when the lookup fails. Callers must keep their default
	// behavior when known is false, so a missing RBAC grant or a slow start
	// never changes how pods are mutated.
	Present(name string) (present bool, known bool)
}

type informerCSIDriverChecker struct {
	lister storagelisters.CSIDriverLister
	synced cache.InformerSynced
}

// NewCSIDriverChecker returns a CSIDriverChecker backed by the given
// informer. The informer must be started by the caller's informer factory.
func NewCSIDriverChecker(informer storageinformers.CSIDriverInformer) CSIDriverChecker {
	return &informerCSIDriverChecker{
		lister: informer.Lister(),
		synced: informer.Informer().HasSynced,
	}
}

func (c *informerCSIDriverChecker) Present(name string) (bool, bool) {
	if !c.synced() {
		return false, false
	}
	_, err := c.lister.Get(name)
	switch {
	case err == nil:
		return true, true
	case apierrors.IsNotFound(err):
		return false, true
	default:
		klog.Warningf("Could not look up CSIDriver %s: %v", name, err)
		return false, false
	}
}
