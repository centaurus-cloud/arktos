/*
Copyright 2016 The Kubernetes Authors.
Copyright 2020 Authors of Arktos - file modified.

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

package etcd3

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apiserver/pkg/storage/datapartition"
	"k8s.io/apiserver/pkg/storage/storagecluster"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"go.etcd.io/etcd/clientv3"
	"k8s.io/klog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/features"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/etcd3/metrics"
	"k8s.io/apiserver/pkg/storage/value"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	utiltrace "k8s.io/utils/trace"
)

// authenticatedDataString satisfies the value.Context interface. It uses the key to
// authenticate the stored data. This does not defend against reuse of previously
// encrypted values under the same key, but will prevent an attacker from using an
// encrypted value from a different key. A stronger authenticated data segment would
// include the etcd3 Version field (which is incremented on each write to a key and
// reset when the key is deleted), but an attacker with write access to etcd can
// force deletion and recreation of keys to weaken that angle.
type authenticatedDataString string

// AuthenticatedData implements the value.Context interface.
func (d authenticatedDataString) AuthenticatedData() []byte {
	return []byte(string(d))
}

var _ value.Context = authenticatedDataString("")

type store struct {
	client *clientv3.Client
	// map from cluster id to etcd client
	dataClusterClients     map[uint8]*clientv3.Client
	dataClusterDestroyFunc map[uint8]func()
	dataClientAddCh        chan uint8

	// getOpts contains additional options that should be passed
	// to all Get() calls.
	getOps              []clientv3.OpOption
	codec               runtime.Codec
	versioner           storage.Versioner
	transformer         value.Transformer
	pathPrefix          string
	watcher             *watcher
	dataClusterWatchers map[uint8]*watcher

	pagingEnabled bool
	leaseManager  *leaseManager

	partitionConfigMap map[string]storage.Interval

	dataClientMux sync.Mutex
}

type objState struct {
	obj   runtime.Object
	meta  *storage.ResponseMeta
	rev   int64
	data  []byte
	stale bool
}

// New returns an etcd3 implementation of storage.Interface.
func New(c *clientv3.Client, codec runtime.Codec, prefix string, transformer value.Transformer, pagingEnabled bool) storage.StorageClusterInterface {
	return newStoreWithPartitionConfig(c, pagingEnabled, codec, prefix, transformer, nil)
}

// New returns an etcd3 implementation of storage.Interface with partition config
func NewWithPartitionConfig(c *clientv3.Client, codec runtime.Codec, prefix string, transformer value.Transformer, pagingEnabled bool, partitionConfigMap map[string]storage.Interval) storage.StorageClusterInterface {
	return newStoreWithPartitionConfig(c, pagingEnabled, codec, prefix, transformer, partitionConfigMap)
}

func newStore(c *clientv3.Client, pagingEnabled bool, codec runtime.Codec, prefix string, transformer value.Transformer) *store {
	return newStoreWithPartitionConfig(c, pagingEnabled, codec, prefix, transformer, nil)
}

func newStoreWithPartitionConfig(c *clientv3.Client, pagingEnabled bool, codec runtime.Codec, prefix string, transformer value.Transformer, partitionConfigMap map[string]storage.Interval) *store {
	versioner := APIObjectVersioner{}
	updatePartitionChGrp := datapartition.GetDataPartitionUpdateChGrp()
	updatePartitionCh := updatePartitionChGrp.Join()

	result := &store{
		client:                 c,
		dataClusterClients:     make(map[uint8]*clientv3.Client),
		dataClusterDestroyFunc: make(map[uint8]func()),
		dataClientAddCh:        make(chan uint8),
		codec:                  codec,
		versioner:              versioner,
		transformer:            transformer,
		pagingEnabled:          pagingEnabled,
		partitionConfigMap:     partitionConfigMap,
		// for compatibility with etcd2 impl.
		// no-op for default prefix of '/registry'.
		// keeps compatibility with etcd2 impl for custom prefixes that don't start with '/'
		pathPrefix:          path.Join("/", prefix),
		watcher:             newWatcherWithPartitionConfig(c, codec, versioner, transformer, partitionConfigMap, updatePartitionCh),
		dataClusterWatchers: make(map[uint8]*watcher),
		leaseManager:        newDefaultLeaseManager(c),
	}
	return result
}

func (s *store) AddDataClient(c *clientv3.Client, clusterId uint8, destroyFunc func()) error {
	s.dataClientMux.Lock()
	defer s.dataClientMux.Unlock()

	existingClient, isOK := s.dataClusterClients[clusterId]
	if isOK {
		err := errors.New(fmt.Sprintf("Trying to add client for existed storage cluster id %d, endpoints [%+v]. Skipping", clusterId, existingClient.Endpoints()))
		return err
	}

	s.dataClusterClients[clusterId] = c
	s.dataClusterDestroyFunc[clusterId] = destroyFunc

	updatePartitionChGrp := datapartition.GetDataPartitionUpdateChGrp()
	updatePartitionCh := updatePartitionChGrp.Join()

	newWatcher := newWatcherWithPartitionConfig(c, s.codec, s.versioner, s.transformer, s.partitionConfigMap, updatePartitionCh)
	s.dataClusterWatchers[clusterId] = newWatcher

	s.dataClientAddCh <- clusterId

	klog.V(3).Infof("Added new data client with cluster id %d, endpoints [%+v]", clusterId, c.Endpoints())
	return nil
}

func (s *store) UpdateDataClient(c *clientv3.Client, clusterId uint8, destroyFunc func()) error {
	s.dataClientMux.Lock()

	existingClient, isOK := s.dataClusterClients[clusterId]
	if !isOK {
		s.dataClientMux.Unlock()
		klog.Warningf("Expected cluster %d not found in data client map. Adding data client %v", clusterId, c.Endpoints())
		return s.AddDataClient(c, clusterId, destroyFunc)
	}
	if reflect.DeepEqual(existingClient.Endpoints(), c.Endpoints()) {
		s.dataClientMux.Unlock()
		klog.Infof("Cluster %d does not have endpoints update. Skip updating. Endpoint %v", clusterId, c.Endpoints())
		return nil
	}

	// stop old client
	existingDestroyFunc, isOK := s.dataClusterDestroyFunc[clusterId]
	if isOK {
		existingDestroyFunc()
	} else {
		klog.Warningf("Previous cluster %d did not have destroy func. Skip destroying. Endpoints %v", clusterId, existingClient.Endpoints())
	}

	s.dataClusterClients[clusterId] = c
	s.dataClusterDestroyFunc[clusterId] = destroyFunc
	klog.V(3).Infof("Updated data client for cluster id %d, endpoints [%+v]", clusterId, c.Endpoints())
	s.dataClientMux.Unlock()
	return nil
}

func (s *store) DeleteDataClient(clusterId uint8) {
	s.dataClientMux.Lock()
	defer s.dataClientMux.Unlock()

	client, isOK := s.dataClusterClients[clusterId]
	if isOK {
		existingDestroyFunc, isOK := s.dataClusterDestroyFunc[clusterId]
		if isOK {
			existingDestroyFunc()
		} else {
			klog.Warningf("Previous cluster %d did not have destroy func. Skip destroying. Endpoints %v", clusterId, client.Endpoints())
		}

		delete(s.dataClusterClients, clusterId)
		delete(s.dataClusterDestroyFunc, clusterId)
		klog.V(3).Infof("Deleted data client for cluster id %d, endpoints [%+v]", clusterId, client.Endpoints())
	} else {
		klog.V(3).Infof("Cluster id %d does not have data client. Skip deleting.", clusterId)
	}
}

// Versioner implements storage.Interface.Versioner.
func (s *store) Versioner() storage.Versioner {
	return s.versioner
}

// Get implements storage.Interface.Get.
func (s *store) Get(ctx context.Context, key string, resourceVersion string, out runtime.Object, ignoreNotFound bool) error {
	key = path.Join(s.pathPrefix, key)
	tenant, _ := genericapirequest.TenantFrom(ctx)
	startTime := time.Now()
	getResp, err := s.getClientFromTenant(tenant).KV.Get(ctx, key, s.getOps...)
	metrics.RecordEtcdRequestLatency("get", getTypeName(out), startTime)
	if err != nil {
		return err
	}

	if len(getResp.Kvs) == 0 {
		if ignoreNotFound {
			return runtime.SetZeroValue(out)
		}
		return storage.NewKeyNotFoundError(key, 0)
	}
	kv := getResp.Kvs[0]

	data, _, err := s.transformer.TransformFromStorage(kv.Value, authenticatedDataString(key))

	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	return decode(s.codec, s.versioner, data, out, kv.ModRevision)
}

// Create implements storage.Interface.Create.
func (s *store) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	if version, err := s.versioner.ObjectResourceVersion(obj); err == nil && version != 0 {
		return errors.New("resourceVersion should not be set on objects to be created")
	}
	if err := s.versioner.PrepareObjectForStorage(obj); err != nil {
		return fmt.Errorf("PrepareObjectForStorage failed: %v", err)
	}
	data, err := runtime.Encode(s.codec, obj)
	if err != nil {
		return err
	}
	key = path.Join(s.pathPrefix, key)

	opts, err := s.ttlOpts(ctx, int64(ttl))
	if err != nil {
		return err
	}

	newData, err := s.transformer.TransformToStorage(data, authenticatedDataString(key))
	if err != nil {
		return storage.NewInternalError(err.Error())
	}

	tenant, _ := genericapirequest.TenantFrom(ctx)
	startTime := time.Now()
	txnResp, err := s.getClientFromTenant(tenant).KV.Txn(ctx).If(
		notFound(key),
	).Then(
		clientv3.OpPut(key, string(newData), opts...),
	).Commit()
	metrics.RecordEtcdRequestLatency("create", getTypeName(obj), startTime)
	if err != nil {
		return err
	}
	if !txnResp.Succeeded {
		return storage.NewKeyExistsError(key, 0)
	}

	if out != nil {
		putResp := txnResp.Responses[0].GetResponsePut()
		return decode(s.codec, s.versioner, data, out, putResp.Header.Revision)
	}
	return nil
}

// Delete implements storage.Interface.Delete.
func (s *store) Delete(ctx context.Context, key string, out runtime.Object, preconditions *storage.Preconditions, validateDeletion storage.ValidateObjectFunc) error {
	v, err := conversion.EnforcePtr(out)
	if err != nil {
		panic("unable to convert output object to pointer")
	}
	key = path.Join(s.pathPrefix, key)
	return s.conditionalDelete(ctx, key, out, v, preconditions, validateDeletion)
}

func (s *store) conditionalDelete(ctx context.Context, key string, out runtime.Object, v reflect.Value, preconditions *storage.Preconditions, validateDeletion storage.ValidateObjectFunc) error {
	tenant, _ := genericapirequest.TenantFrom(ctx)
	startTime := time.Now()
	getResp, err := s.getClientFromTenant(tenant).KV.Get(ctx, key)
	metrics.RecordEtcdRequestLatency("get", getTypeName(out), startTime)
	if err != nil {
		return err
	}
	for {
		origState, err := s.getState(getResp, key, v, false)
		if err != nil {
			return err
		}
		if preconditions != nil {
			if err := preconditions.Check(key, origState.obj); err != nil {
				return err
			}
		}
		if err := validateDeletion(origState.obj); err != nil {
			return err
		}
		startTime := time.Now()
		txnResp, err := s.getClientFromTenant(tenant).KV.Txn(ctx).If(
			clientv3.Compare(clientv3.ModRevision(key), "=", origState.rev),
		).Then(
			clientv3.OpDelete(key),
		).Else(
			clientv3.OpGet(key),
		).Commit()
		metrics.RecordEtcdRequestLatency("delete", getTypeName(out), startTime)
		if err != nil {
			return err
		}
		if !txnResp.Succeeded {
			getResp = (*clientv3.GetResponse)(txnResp.Responses[0].GetResponseRange())
			klog.V(4).Infof("deletion of %s failed because of a conflict, going to retry", key)
			continue
		}
		return decode(s.codec, s.versioner, origState.data, out, origState.rev)
	}
}

// GuaranteedUpdate implements storage.Interface.GuaranteedUpdate.
func (s *store) GuaranteedUpdate(
	ctx context.Context, key string, out runtime.Object, ignoreNotFound bool,
	preconditions *storage.Preconditions, tryUpdate storage.UpdateFunc, suggestion ...runtime.Object) error {
	trace := utiltrace.New(fmt.Sprintf("GuaranteedUpdate etcd3: %s", getTypeName(out)))
	defer trace.LogIfLong(500 * time.Millisecond)

	v, err := conversion.EnforcePtr(out)
	if err != nil {
		panic("unable to convert output object to pointer")
	}
	key = path.Join(s.pathPrefix, key)

	tenant, _ := genericapirequest.TenantFrom(ctx)
	getCurrentState := func() (*objState, error) {
		startTime := time.Now()
		getResp, err := s.getClientFromTenant(tenant).KV.Get(ctx, key, s.getOps...)
		metrics.RecordEtcdRequestLatency("get", getTypeName(out), startTime)
		if err != nil {
			return nil, err
		}
		return s.getState(getResp, key, v, ignoreNotFound)
	}

	var origState *objState
	var mustCheckData bool
	if len(suggestion) == 1 && suggestion[0] != nil {
		origState, err = s.getStateFromObject(suggestion[0])
		if err != nil {
			return err
		}
		mustCheckData = true
	} else {
		origState, err = getCurrentState()
		if err != nil {
			return err
		}
	}
	trace.Step("initial value restored")

	transformContext := authenticatedDataString(key)
	for {
		if err := preconditions.Check(key, origState.obj); err != nil {
			// If our data is already up to date, return the error
			if !mustCheckData {
				return err
			}

			// It's possible we were working with stale data
			// Actually fetch
			origState, err = getCurrentState()
			if err != nil {
				return err
			}
			mustCheckData = false
			// Retry
			continue
		}

		ret, ttl, updatettl, err := s.updateState(origState, tryUpdate)
		if err != nil {
			// If our data is already up to date, return the error
			if !mustCheckData {
				return err
			}

			// It's possible we were working with stale data
			// Actually fetch
			origState, err = getCurrentState()
			if err != nil {
				return err
			}
			mustCheckData = false
			// Retry
			continue
		}

		data, err := runtime.Encode(s.codec, ret)
		if err != nil {
			return err
		}

		v1 := bytes.Equal(data, origState.data)
		if !origState.stale && v1 {
			// if we skipped the original Get in this loop, we must refresh from
			// etcd in order to be sure the data in the store is equivalent to
			// our desired serialization
			if mustCheckData {
				origState, err = getCurrentState()
				if err != nil {
					return err
				}
				mustCheckData = false
				if !bytes.Equal(data, origState.data) {
					// original data changed, restart loop
					continue
				}
			}
			// recheck that the data from etcd is not stale before short-circuiting a write
			if !origState.stale && updatettl == 0 {
				return decode(s.codec, s.versioner, origState.data, out, origState.rev)
			}
		}

		newData, err := s.transformer.TransformToStorage(data, transformContext)
		if err != nil {
			return storage.NewInternalError(err.Error())
		}

		opts, err := s.ttlOpts(ctx, int64(ttl))
		if err != nil {
			return err
		}

		if updatettl > 0 {
			opts, err = s.ttlOpts(ctx, int64(updatettl))
			if err != nil {
				return err
			}
		}
		trace.Step("Transaction prepared")

		tenant, _ := genericapirequest.TenantFrom(ctx)
		startTime := time.Now()
		txnResp, err := s.getClientFromTenant(tenant).KV.Txn(ctx).If(
			clientv3.Compare(clientv3.ModRevision(key), "=", origState.rev),
		).Then(
			clientv3.OpPut(key, string(newData), opts...),
		).Else(
			clientv3.OpGet(key),
		).Commit()
		metrics.RecordEtcdRequestLatency("update", getTypeName(out), startTime)
		if err != nil {
			return err
		}
		trace.Step("Transaction committed")
		if !txnResp.Succeeded {
			getResp := (*clientv3.GetResponse)(txnResp.Responses[0].GetResponseRange())
			klog.V(4).Infof("GuaranteedUpdate of %s failed because of a conflict, going to retry", key)
			origState, err = s.getState(getResp, key, v, ignoreNotFound)
			if err != nil {
				return err
			}
			trace.Step("Retry value restored")
			mustCheckData = false
			continue
		}
		putResp := txnResp.Responses[0].GetResponsePut()

		trace.Step("Starting decode")
		return decode(s.codec, s.versioner, data, out, putResp.Header.Revision)
	}
}

// GetToList implements storage.Interface.GetToList.
func (s *store) GetToList(ctx context.Context, key string, resourceVersion string, pred storage.SelectionPredicate, listObj runtime.Object) error {
	trace := utiltrace.New(fmt.Sprintf("GetToList etcd3: key=%v, resourceVersion=%s, limit: %d, continue: %s", key, resourceVersion, pred.Limit, pred.Continue))
	defer trace.LogIfLong(500 * time.Millisecond)
	listPtr, err := meta.GetItemsPtr(listObj)
	if err != nil {
		return err
	}
	v, err := conversion.EnforcePtr(listPtr)
	if err != nil || v.Kind() != reflect.Slice {
		panic("need ptr to slice")
	}

	key = path.Join(s.pathPrefix, key)
	tenant, _ := genericapirequest.TenantFrom(ctx)
	startTime := time.Now()
	getResp, err := s.getClientFromTenant(tenant).KV.Get(ctx, key, s.getOps...)
	metrics.RecordEtcdRequestLatency("get", getTypeName(listPtr), startTime)
	if err != nil {
		return err
	}

	trace.Step("Starting transform from storage")
	if len(getResp.Kvs) > 0 {
		data, _, err := s.transformer.TransformFromStorage(getResp.Kvs[0].Value, authenticatedDataString(key))
		if err != nil {
			return storage.NewInternalError(err.Error())
		}
		if err := appendListItem(v, data, uint64(getResp.Kvs[0].ModRevision), pred, s.codec, s.versioner); err != nil {
			return err
		}
	}
	// update version with cluster level revision
	trace.Step("Starting update list")
	err = s.versioner.UpdateList(listObj, uint64(getResp.Header.Revision), "", nil)

	return err
}

func (s *store) Count(key string) (int64, error) {
	key = path.Join(s.pathPrefix, key)
	startTime := time.Now()
	getResp, err := s.client.KV.Get(context.Background(), key, clientv3.WithRange(clientv3.GetPrefixRangeEnd(key)), clientv3.WithCountOnly())
	metrics.RecordEtcdRequestLatency("listWithCount", key, startTime)
	if err != nil {
		return 0, err
	}
	return getResp.Count, nil
}

// continueToken is a simple structured object for encoding the state of a continue token.
// TODO: if we change the version of the encoded from, we can't start encoding the new version
// until all other servers are upgraded (i.e. we need to support rolling schema)
// This is a public API struct and cannot change.
type continueToken struct {
	ClusterId       uint8  `json:"c"`
	APIVersion      string `json:"v"`
	ResourceVersion int64  `json:"rv"`
	StartKey        string `json:"start"`
	continueKey     string `json:"-"`
}

// parseFrom transforms an encoded predicate from into a versioned struct.
// TODO: return a typed error that instructs clients that they must relist
func decodeContinue(continueValue, keyPrefix string) (continueTokens []*continueToken, err error) {
	data, err := base64.RawURLEncoding.DecodeString(continueValue)
	if err != nil {
		return nil, fmt.Errorf("continue key is not valid: %v", err)
	}
	if err := json.Unmarshal(data, &continueTokens); err != nil {
		return nil, fmt.Errorf("continue key is not valid: %v", err)
	}

	foundClusterIds := make(map[uint8]bool, len(continueTokens))
	for _, c := range continueTokens {
		switch c.APIVersion {
		case "meta.k8s.io/v1":
			if c.ResourceVersion == 0 {
				return nil, fmt.Errorf("continue key is not valid: incorrect encoded start resourceVersion (version meta.k8s.io/v1)")
			}
			if len(c.StartKey) == 0 {
				return nil, fmt.Errorf("continue key is not valid: encoded start key empty (version meta.k8s.io/v1)")
			}
			// defend against path traversal attacks by clients - path.Clean will ensure that startKey cannot
			// be at a higher level of the hierarchy, and so when we append the key prefix we will end up with
			// continue start key that is fully qualified and cannot range over anything less specific than
			// keyPrefix.
			key := c.StartKey
			if !strings.HasPrefix(key, "/") {
				key = "/" + key
			}
			cleaned := path.Clean(key)
			if cleaned != key {
				return nil, fmt.Errorf("continue key is not valid: %s", c.StartKey)
			}

			c.continueKey = keyPrefix + cleaned[1:]

			_, ok := foundClusterIds[c.ClusterId]
			if ok {
				return nil, fmt.Errorf("continue key is not valid: duplicated cluster id %v", c.ClusterId)
			}
			foundClusterIds[c.ClusterId] = true
		default:
			return nil, fmt.Errorf("continue key is not valid: server does not recognize this encoded version %q", c.APIVersion)
		}
	}

	return
}

// encodeContinue returns a string representing the encoded continuation of the current query.
func encodeContinue(listResult []listPartitionResult) (string, error) {
	continueTokens := []continueToken{}

	for _, result := range listResult {
		nextKey := strings.TrimPrefix(result.key, result.keyPrefix)
		if nextKey == result.key {
			return "", fmt.Errorf("unable to encode next field: the key and key prefix do not match")
		}

		continueTokens = append(continueTokens,
			continueToken{
				ClusterId:       result.clusterId,
				APIVersion:      "meta.k8s.io/v1",
				ResourceVersion: result.returnedRV,
				StartKey:        nextKey,
			})
	}

	out, err := json.Marshal(&continueTokens)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(out), nil
}

// List implements storage.Interface.List.
// Currently does not support continue for multiple storage cluster list - TODO
func (s *store) List(ctx context.Context, key, resourceVersion string, pred storage.SelectionPredicate, listObj runtime.Object) error {
	trace := utiltrace.New(fmt.Sprintf("List etcd3: key=%v, resourceVersion=%s, limit: %d, continue: %s", key, resourceVersion, pred.Limit, pred.Continue))
	defer trace.LogIfLong(500 * time.Millisecond)
	listPtr, err := meta.GetItemsPtr(listObj)
	if err != nil {
		return err
	}
	v, err := conversion.EnforcePtr(listPtr)
	if err != nil || v.Kind() != reflect.Slice {
		panic("need ptr to slice")
	}

	if s.pathPrefix != "" {
		key = path.Join(s.pathPrefix, key)
	}
	// We need to make sure the key ended with "/" so that we only get children "directories".
	// e.g. if we have key "/a", "/a/b", "/ab", getting keys with prefix "/a" will return all three,
	// while with prefix "/a/" will return only "/a/b" which is the correct answer.
	if !strings.HasSuffix(key, "/") {
		key += "/"
	}
	keyPrefix := key
	tenant, _ := genericapirequest.TenantFrom(ctx)

	// set the appropriate clientv3 options to filter the returned data set
	var paging bool
	options := make([]clientv3.OpOption, 0, 4)
	if s.pagingEnabled && pred.Limit > 0 {
		paging = true
		options = append(options, clientv3.WithLimit(pred.Limit))
	}

	var clients map[uint8]*clientv3.Client

	var continueTokens []*continueToken
	var returnedRV int64
	switch {
	case s.pagingEnabled && len(pred.Continue) > 0:
		if len(resourceVersion) > 0 && resourceVersion != "0" {
			return apierrors.NewBadRequest("specifying resource version is not allowed when using continue")
		}

		continueTokens, err = decodeContinue(pred.Continue, keyPrefix)
		if err != nil {
			return apierrors.NewBadRequest(fmt.Sprintf("invalid continue token: %v", err))
		}
		rangeEnd := clientv3.GetPrefixRangeEnd(keyPrefix)
		options = append(options, clientv3.WithRange(rangeEnd))

		// get clients
		clients = make(map[uint8]*clientv3.Client, len(continueTokens))
		if len(continueTokens) == 1 && continueTokens[0].ResourceVersion == -1 {
			clients = s.getClientsFromKey(key, tenant)
		} else {
			for clusterId := range continueTokens {
				if clusterId == 0 {
					clients[0] = s.client
				} else {
					client, ok := s.dataClusterClients[uint8(clusterId)]
					if !ok {
						return apierrors.NewBadRequest(fmt.Sprintf("invalid continue token: %v. clusterId %v does not exist", err, clusterId))
					}
					clients[uint8(clusterId)] = client
				}
			}
		}
	case s.pagingEnabled && pred.Limit > 0:
		if len(resourceVersion) > 0 {
			fromRV, err := s.versioner.ParseResourceVersion(resourceVersion)
			if err != nil {
				return apierrors.NewBadRequest(fmt.Sprintf("invalid resource version: %v", err))
			}
			if fromRV > 0 {
				options = append(options, clientv3.WithRev(int64(fromRV)))
			}
			returnedRV = int64(fromRV)
		}

		rangeEnd := clientv3.GetPrefixRangeEnd(keyPrefix)
		options = append(options, clientv3.WithRange(rangeEnd))

		clients = s.getClientsFromKey(key, tenant)
	default:
		if len(resourceVersion) > 0 {
			fromRV, err := s.versioner.ParseResourceVersion(resourceVersion)
			if err != nil {
				return apierrors.NewBadRequest(fmt.Sprintf("invalid resource version: %v", err))
			}
			if fromRV > 0 {
				options = append(options, clientv3.WithRev(int64(fromRV)))
			}
			returnedRV = int64(fromRV)
		}

		options = append(options, clientv3.WithPrefix())
		clients = s.getClientsFromKey(key, tenant)
	}

	listResults := make(map[uint8]*listPartitionResult, len(clients))
	var wg sync.WaitGroup
	wg.Add(len(clients))
	var listAppendMux sync.Mutex

	trace.Step("Starting list from etcd")
	for i, c := range clients {
		var optionsToUse []clientv3.OpOption
		keyToUse := key
		continueKey := ""

		if s.pagingEnabled && len(pred.Continue) > 0 {
			keyToUse = continueTokens[i].continueKey
			optionsToUse = append(optionsToUse, options...)

			// If continueRV > 0, the LIST request needs a specific resource version.
			// continueRV==0 is invalid.
			// If continueRV < 0, the request is for the latest resource version.
			if continueTokens[i].ResourceVersion > 0 {
				optionsToUse = append(optionsToUse, clientv3.WithRev(continueTokens[i].ResourceVersion))
				returnedRV = continueTokens[i].ResourceVersion
				continueKey = continueTokens[i].continueKey
			}
		} else {
			optionsToUse = options
		}

		go func(c *clientv3.Client, key string, returnedRV int64, continueKey string, options []clientv3.OpOption,
			listResults map[uint8]*listPartitionResult, i uint8, listMux *sync.Mutex) {
			klog.V(6).Infof("List key %s from multi partitions. client %d, endpoint %v, paging [%v], returnedRV [%v], continueKey [%v], keyPrefix [%v], resourceVersion [%v]",
				key, i, c.Endpoints(), paging, returnedRV, continueKey, keyPrefix, resourceVersion)
			result, err := s.listPartition(ctx, c, key, pred, listObj, options, paging, returnedRV, continueKey, keyPrefix, listMux)
			if err != nil {
				result = &listPartitionResult{err: err}
			}
			listMux.Lock()
			listResults[i] = result
			listMux.Unlock()
			wg.Done()
		}(c, keyToUse, returnedRV, continueKey, optionsToUse, listResults, i, &listAppendMux)
	}

	wg.Wait()
	klog.V(4).Infof("list partition completed. len(clients) = %v", len(clients))
	return s.updatelist(listObj, listResults)
}

type listPartitionResult struct {
	clusterId          uint8
	err                error
	returnedRV         int64
	hasNext            bool
	remainingItemCount *int64
	key                string
	keyPrefix          string
}

func (s *store) updatelist(listObj runtime.Object, listResult map[uint8]*listPartitionResult) error {
	for _, result := range listResult {
		if result.err != nil {
			return result.err
		}
	}

	returnedRV := int64(0)
	hasNext := false
	var remainingItemCount *int64
	compactedListResult := []listPartitionResult{}

	for clusterId, result := range listResult {
		if diff.RevisionIsNewer(uint64(result.returnedRV), uint64(returnedRV)) {
			returnedRV = result.returnedRV
		}

		if result.hasNext {
			hasNext = true
			result.clusterId = clusterId
			if result.remainingItemCount != nil {
				if remainingItemCount == nil {
					remainingItemCount = result.remainingItemCount
				} else {
					total := *remainingItemCount + *result.remainingItemCount
					remainingItemCount = &total
				}
			}
			compactedListResult = append(compactedListResult, *result)
		}
	}

	if hasNext {
		nextKeyEncoded, err := encodeContinue(compactedListResult)
		if err != nil {
			return err
		}
		return s.versioner.UpdateList(listObj, uint64(returnedRV), nextKeyEncoded, remainingItemCount)
	}

	return s.versioner.UpdateList(listObj, uint64(returnedRV), "", nil)
}

func (s *store) listPartition(ctx context.Context, client *clientv3.Client, key string, pred storage.SelectionPredicate, listObj runtime.Object,
	options []clientv3.OpOption, paging bool, returnedRV int64, continueKey string, keyPrefix string, listAppendMux *sync.Mutex) (*listPartitionResult, error) {

	trace := utiltrace.New(fmt.Sprintf("listPartition etcd3: key=%s", key))
	defer trace.LogIfLong(500 * time.Millisecond)

	result := &listPartitionResult{
		err:                nil,
		returnedRV:         returnedRV,
		hasNext:            false,
		remainingItemCount: nil,
	}

	// error checked in List()
	listPtr, _ := meta.GetItemsPtr(listObj)
	v, _ := conversion.EnforcePtr(listPtr)

	// loop until we have filled the requested limit from etcd or there are no more results
	var lastKey []byte
	var hasMore bool
	var getResp *clientv3.GetResponse
	var err error
	for {
		startTime := time.Now()
		getResp, err = client.KV.Get(ctx, key, options...)
		metrics.RecordEtcdRequestLatency("list", getTypeName(listPtr), startTime)
		if err != nil {
			return nil, interpretListError(err, len(pred.Continue) > 0, continueKey, keyPrefix)
		}
		hasMore = getResp.More

		if len(getResp.Kvs) == 0 && getResp.More {
			return nil, fmt.Errorf("no results were found, but etcd indicated there were more values remaining")
		}

		// avoid small allocations for the result slice, since this can be called in many
		// different contexts and we don't know how significantly the result will be filtered
		trace.Step("Acquiring ListAppendMux listPartition")
		listAppendMux.Lock()
		trace.Step("Acquired ListAppendMux listPartition")

		if pred.Empty() {
			growSlice(v, len(getResp.Kvs))
		} else {
			growSlice(v, 2048, len(getResp.Kvs))
		}

		// take items from the response until the bucket is full, filtering as we go
		for _, kv := range getResp.Kvs {
			if paging && int64(v.Len()) >= pred.Limit {
				hasMore = true
				break
			}
			lastKey = kv.Key

			data, _, err := s.transformer.TransformFromStorage(kv.Value, authenticatedDataString(kv.Key))
			if err != nil {
				listAppendMux.Unlock()
				return nil, storage.NewInternalErrorf("unable to transform key %q: %v", kv.Key, err)
			}

			if err := appendListItem(v, data, uint64(kv.ModRevision), pred, s.codec, s.versioner); err != nil {
				listAppendMux.Unlock()
				return nil, err
			}
		}
		listAppendMux.Unlock()
		trace.Step("Took items from the response")

		// indicate to the client which resource version was returned
		if returnedRV == 0 {
			returnedRV = getResp.Header.Revision
		}

		// no more results remain or we didn't request paging
		if !hasMore || !paging {
			break
		}
		// we're paging but we have filled our bucket
		if int64(v.Len()) >= pred.Limit {
			break
		}
		key = string(lastKey) + "\x00"
	}

	// instruct the client to begin querying from immediately after the last key we returned
	// we never return a key that the client wouldn't be allowed to see
	if hasMore {
		// we want to start immediately after the last key
		result.key = string(lastKey) + "\x00"
		result.keyPrefix = keyPrefix
		result.returnedRV = returnedRV
		result.hasNext = true
		trace.Step("Has more " + result.key)

		// getResp.Count counts in objects that do not match the pred.
		// Instead of returning inaccurate count for non-empty selectors, we return nil.
		// Only set remainingItemCount if the predicate is empty.
		if utilfeature.DefaultFeatureGate.Enabled(features.RemainingItemCount) {
			if pred.Empty() {
				c := int64(getResp.Count - pred.Limit)
				result.remainingItemCount = &c
			}
		}
		return result, nil
		//return s.versioner.UpdateList(listObj, uint64(returnedRV), next, remainingItemCount), lastRV
	}

	// no continuation
	result.returnedRV = returnedRV
	return result, nil
	//return s.versioner.UpdateList(listObj, uint64(returnedRV), "", nil), lastRV
}

// growSlice takes a slice value and grows its capacity up
// to the maximum of the passed sizes or maxCapacity, whichever
// is smaller. Above maxCapacity decisions about allocation are left
// to the Go runtime on append. This allows a caller to make an
// educated guess about the potential size of the total list while
// still avoiding overly aggressive initial allocation. If sizes
// is empty maxCapacity will be used as the size to grow.
func growSlice(v reflect.Value, maxCapacity int, sizes ...int) {
	cap := v.Cap()
	max := cap
	for _, size := range sizes {
		if size > max {
			max = size
		}
	}
	if len(sizes) == 0 || max > maxCapacity {
		max = maxCapacity
	}
	if max <= cap {
		return
	}
	if v.Len() > 0 {
		extra := reflect.MakeSlice(v.Type(), 0, max)
		reflect.Copy(extra, v)
		v.Set(extra)
	} else {
		extra := reflect.MakeSlice(v.Type(), 0, max)
		v.Set(extra)
	}
}

// Watch implements storage.Interface.Watch.
func (s *store) Watch(ctx context.Context, key string, resourceVersion string, pred storage.SelectionPredicate) (watch.Interface, error) {
	//klog.Infof("=====etcd3 store watch key %s", key)
	return s.watch(ctx, key, resourceVersion, pred, false)
}

// WatchList implements storage.Interface.WatchList.
func (s *store) WatchList(ctx context.Context, key string, resourceVersion string, pred storage.SelectionPredicate) (watch.Interface, error) {
	//klog.Infof("=====etcd3 store watchlist key %s", key)
	return s.watch(ctx, key, resourceVersion, pred, true)
}

func (s *store) watch(ctx context.Context, key string, rv string, pred storage.SelectionPredicate, recursive bool) (watch.Interface, error) {
	rev, err := s.versioner.ParseResourceVersion(rv)
	if err != nil {
		return nil, err
	}
	key = path.Join(s.pathPrefix, key)
	aw := s.watcher.Watch(ctx, key, int64(rev), recursive, pred)

	// TODO - update revision at new watch start
	go func(s *store, ctx context.Context, key string, rv string, pred storage.SelectionPredicate, recursive bool, aw watch.AggregatedWatchInterface) {
		// Waiting for storage cluster updates
		for newClusterId := range s.dataClientAddCh {
			s.dataClientMux.Lock()
			newWatcher := s.dataClusterWatchers[newClusterId]
			nw := newWatcher.Watch(ctx, key, int64(rev), recursive, pred)
			aw.AddWatchInterface(nw, nw.GetErrors())
			s.dataClientMux.Unlock()
		}
	}(s, ctx, key, rv, pred, recursive, aw)

	return aw, aw.GetErrors()
}

func (s *store) getState(getResp *clientv3.GetResponse, key string, v reflect.Value, ignoreNotFound bool) (*objState, error) {
	state := &objState{
		meta: &storage.ResponseMeta{},
	}

	if u, ok := v.Addr().Interface().(runtime.Unstructured); ok {
		state.obj = u.NewEmptyInstance()
	} else {
		state.obj = reflect.New(v.Type()).Interface().(runtime.Object)
	}

	if len(getResp.Kvs) == 0 {
		if !ignoreNotFound {
			return nil, storage.NewKeyNotFoundError(key, 0)
		}
		if err := runtime.SetZeroValue(state.obj); err != nil {
			return nil, err
		}
	} else {
		data, stale, err := s.transformer.TransformFromStorage(getResp.Kvs[0].Value, authenticatedDataString(key))
		if err != nil {
			return nil, storage.NewInternalError(err.Error())
		}
		state.rev = getResp.Kvs[0].ModRevision
		state.meta.ResourceVersion = uint64(state.rev)
		state.data = data
		state.stale = stale
		if err := decode(s.codec, s.versioner, state.data, state.obj, state.rev); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (s *store) getStateFromObject(obj runtime.Object) (*objState, error) {
	state := &objState{
		obj:  obj,
		meta: &storage.ResponseMeta{},
	}

	rv, err := s.versioner.ObjectResourceVersion(obj)
	if err != nil {
		return nil, fmt.Errorf("couldn't get resource version: %v", err)
	}
	state.rev = int64(rv)
	state.meta.ResourceVersion = uint64(state.rev)

	// Compute the serialized form - for that we need to temporarily clean
	// its resource version field (those are not stored in etcd).
	if err := s.versioner.PrepareObjectForStorage(obj); err != nil {
		return nil, fmt.Errorf("PrepareObjectForStorage failed: %v", err)
	}
	state.data, err = runtime.Encode(s.codec, obj)
	if err != nil {
		return nil, err
	}
	s.versioner.UpdateObject(state.obj, uint64(rv))
	return state, nil
}

func (s *store) updateState(st *objState, userUpdate storage.UpdateFunc) (runtime.Object, uint64, uint64, error) {
	ret, ttlPtr, updateTtlPtr, err := userUpdate(st.obj, *st.meta)
	if err != nil {
		return nil, 0, 0, err
	}

	if err := s.versioner.PrepareObjectForStorage(ret); err != nil {
		return nil, 0, 0, fmt.Errorf("PrepareObjectForStorage failed: %v", err)
	}
	var ttl uint64
	if ttlPtr != nil {
		ttl = *ttlPtr
	}

	var updateTtl uint64
	if updateTtlPtr != nil {
		updateTtl = *updateTtlPtr
	}
	return ret, ttl, updateTtl, nil
}

// ttlOpts returns client options based on given ttl.
// ttl: if ttl is non-zero, it will attach the key to a lease with ttl of roughly the same length
func (s *store) ttlOpts(ctx context.Context, ttl int64) ([]clientv3.OpOption, error) {
	if ttl == 0 {
		return nil, nil
	}
	id, err := s.leaseManager.GetLease(ctx, ttl)
	if err != nil {
		return nil, err
	}
	return []clientv3.OpOption{clientv3.WithLease(id)}, nil
}

// If key prefix matches the following, the 4th segment will be tenant
var regexPrefixToCheck = []string{
	"services/specs/",
	"services/endpoints/",
	"apiregistration.k8s.io/apiservices/",
	"apiextensions.k8s.io/customresourcedefinitions/",
}

// Based on the key tree structure, figure out which client it needs to go to
// If there is no data cluster, all goes to system cluster
// Aside from pathPrefix,
//
// 1. If the key has <= 2 segments (split by "/", the first segment is ""), it goes to system cluster
// 2. If the key following prefix, the 3th segment will be reported as tenant
//		"services/specs/"
//		"services/endpoints/"
//		"apiregistration.k8s.io/apiservices/"
//		"apiextensions.k8s.io/customresourcedefinitions/"
// 3. For the rest, the 2th segment will be reported as tenant
// 4. If tenant name is not found from tenant map, goes to system cluster
func (s *store) getClientFromTenant(tenant string) *clientv3.Client {
	_, client := s.getClientAndClusterIdFromTenant(tenant)
	return client
}

func (s *store) getClientAndClusterIdFromTenant(tenant string) (uint8, *clientv3.Client) {
	if tenant == "" || tenant == metav1.TenantSystem || tenant == metav1.TenantAllExplicit {
		klog.V(5).Infof("system client: tenant (%s) ", tenant)
		return 0, s.client
	}

	clusterId, c := s.getClientForTenant(tenant)
	klog.V(5).Infof("client %v: tenant (%s). cluster id %v", c.Endpoints(), tenant, clusterId)
	return clusterId, c
}

// Based on different data segment, map to differnet data clients
func (s *store) getClientForTenant(tenant string) (uint8, *clientv3.Client) {
	clusterId := storagecluster.GetClusterIdFromTenantHandler(tenant)
	if clusterId == 0 {
		return 0, s.client
	}
	dataclient, isOK := s.dataClusterClients[clusterId]
	if !isOK {
		klog.Warningf("Cluster %d assigned to tenant %s does not exist. Using system cluster instead", clusterId, tenant)
		return 0, s.client
	}
	return clusterId, dataclient
}

func getTenantForKey(segs []string, posToGet int) string {
	if len(segs) < posToGet+1 {
		return ""
	}
	return segs[posToGet]
}

// getClientsFromKey is used by list to get all related storage clusters
// For example: get pods from all tenants
func (s *store) getClientsFromKey(key, tenant string) map[uint8]*clientv3.Client {
	clientMap := make(map[uint8]*clientv3.Client)

	clusterId, client := s.getClientAndClusterIdFromTenant(tenant)
	clientMap[clusterId] = client
	if clusterId != 0 {
		// This key belongs to a data cluster, no need to check other clusters
		return clientMap
	}

	// TODO: currently paginated list is not supported for multi etcd partition
	// Events can have more than one page - workaround
	// Fix later
	if strings.HasPrefix(key, "/registry/events/") {
		clientMap[0] = client
		return clientMap
	}

	// TODO - check whether key can only be in system cluster
	if len(s.dataClusterClients) > 0 {
		for clusterId, client := range s.dataClusterClients {
			clientMap[clusterId] = client
		}
	}
	return clientMap
}

// decode decodes value of bytes into object. It will also set the object resource version to rev.
// On success, objPtr would be set to the object.
func decode(codec runtime.Codec, versioner storage.Versioner, value []byte, objPtr runtime.Object, rev int64) error {
	if _, err := conversion.EnforcePtr(objPtr); err != nil {
		panic("unable to convert output object to pointer")
	}
	_, _, err := codec.Decode(value, nil, objPtr)
	if err != nil {
		return err
	}
	// being unable to set the version does not prevent the object from being extracted
	versioner.UpdateObject(objPtr, uint64(rev))
	return nil
}

// appendListItem decodes and appends the object (if it passes filter) to v, which must be a slice.
func appendListItem(v reflect.Value, data []byte, rev uint64, pred storage.SelectionPredicate, codec runtime.Codec, versioner storage.Versioner) error {
	obj, _, err := codec.Decode(data, nil, reflect.New(v.Type().Elem()).Interface().(runtime.Object))
	if err != nil {
		return err
	}
	// being unable to set the version does not prevent the object from being extracted
	versioner.UpdateObject(obj, rev)
	if matched, err := pred.Matches(obj); err == nil && matched {
		v.Set(reflect.Append(v, reflect.ValueOf(obj).Elem()))
	}
	return nil
}

func notFound(key string) clientv3.Cmp {
	return clientv3.Compare(clientv3.ModRevision(key), "=", 0)
}

// getTypeName returns type name of an object for reporting purposes.
func getTypeName(obj interface{}) string {
	return reflect.TypeOf(obj).String()
}
