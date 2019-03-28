package tessera

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/ethereum/go-ethereum/private/engine"

	"github.com/ethereum/go-ethereum/private/cache"

	"github.com/ethereum/go-ethereum/params"

	gocache "github.com/patrickmn/go-cache"

	"github.com/ethereum/go-ethereum/common"
)

type tesseraPrivateTxManager struct {
	client *engine.Client
	cache  *gocache.Cache
}

func Is(ptm interface{}) bool {
	_, ok := ptm.(*tesseraPrivateTxManager)
	return ok
}

func New(client *engine.Client) *tesseraPrivateTxManager {
	return &tesseraPrivateTxManager{
		client: client,
		cache:  gocache.New(cache.DefaultExpiration, cache.CleanupInterval),
	}
}

func (t *tesseraPrivateTxManager) submitJSON(method, path string, request interface{}, response interface{}) (int, error) {
	req, err := newOptionalJSONRequest(method, t.client.FullPath(path), request)
	if err != nil {
		return -1, err
	}
	res, err := t.client.HttpClient.Do(req)
	if err != nil {
		return -1, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		body, _ := ioutil.ReadAll(res.Body)
		return res.StatusCode, fmt.Errorf("%d status: %s", res.StatusCode, string(body))
	}
	if err := json.NewDecoder(res.Body).Decode(response); err != nil {
		return res.StatusCode, err
	}
	return res.StatusCode, nil
}

func (t *tesseraPrivateTxManager) Send(data []byte, from string, to []string, extra *engine.ExtraMetadata) (common.EncryptedPayloadHash, error) {
	response := new(sendResponse)
	acMerkleRoot := ""
	if !common.EmptyHash(extra.ACMerkleRoot) {
		acMerkleRoot = extra.ACMerkleRoot.ToBase64()
	}
	if _, err := t.submitJSON("POST", "/send", &sendRequest{
		Payload:                      data,
		From:                         from,
		To:                           to,
		AffectedContractTransactions: extra.ACHashes.ToBase64s(),
		ExecHash:                     acMerkleRoot,
	}, response); err != nil {
		return common.EncryptedPayloadHash{}, err
	}

	eph, err := common.Base64ToEncryptedPayloadHash(response.Key)
	if err != nil {
		return common.EncryptedPayloadHash{}, err
	}

	cacheKey := eph.Hex()
	t.cache.Set(cacheKey, cache.PrivateCacheItem{
		Payload: data,
		Extra:   *extra,
	}, gocache.DefaultExpiration)

	return eph, nil
}

func (t *tesseraPrivateTxManager) SendSignedTx(data common.EncryptedPayloadHash, to []string, extra *engine.ExtraMetadata) ([]byte, error) {
	response := new(sendSignedTxResponse)
	acMerkleRoot := ""
	if !common.EmptyHash(extra.ACMerkleRoot) {
		acMerkleRoot = extra.ACMerkleRoot.ToBase64()
	}
	if _, err := t.submitJSON("POST", "/sendsignedtx", &sendSignedTxRequest{
		Hash:                         data.Bytes(),
		To:                           to,
		AffectedContractTransactions: extra.ACHashes.ToBase64s(),
		ExecHash:                     acMerkleRoot,
	}, response); err != nil {
		return nil, err
	}

	hashBytes, err := base64.StdEncoding.DecodeString(response.Key)
	if err != nil {
		return nil, err
	}
	return hashBytes, err
}

func (t *tesseraPrivateTxManager) Receive(data common.EncryptedPayloadHash) ([]byte, *engine.ExtraMetadata, error) {
	return t.receive(data, false)
}

func (t *tesseraPrivateTxManager) ReceiveRaw(data common.EncryptedPayloadHash) ([]byte, *engine.ExtraMetadata, error) {
	return t.receive(data, true)
}

func (t *tesseraPrivateTxManager) receive(data common.EncryptedPayloadHash, isRaw bool) ([]byte, *engine.ExtraMetadata, error) {
	if common.EmptyEncryptedPayloadHash(data) {
		return data.Bytes(), nil, nil
	}
	cacheKey := data.Hex()
	if isRaw {
		cacheKey = fmt.Sprintf("%s-raw", cacheKey)
	}
	if item, found := t.cache.Get(cacheKey); found {
		cacheItem, ok := item.(cache.PrivateCacheItem)
		if !ok {
			return nil, nil, fmt.Errorf("unknown cache item. expected type PrivateCacheItem")
		}
		return cacheItem.Payload, &cacheItem.Extra, nil
	}

	response := new(receiveResponse)
	if statusCode, err := t.submitJSON("GET", fmt.Sprintf("/transaction/%s?isRaw=%v", url.PathEscape(data.ToBase64()), isRaw), nil, response); err != nil {
		if statusCode == http.StatusNotFound {
			return nil, nil, nil
		} else {
			return nil, nil, err
		}
	}

	acHashes, err := common.Base64sToEncryptedPayloadHashes(response.AffectedContractTransactions)
	if err != nil {
		return nil, nil, err
	}
	acMerkleRoot, err := common.Base64ToHash(response.ExecHash)
	if err != nil {
		return nil, nil, err
	}
	extra := &engine.ExtraMetadata{
		ACHashes:     acHashes,
		ACMerkleRoot: acMerkleRoot,
	}

	t.cache.Set(cacheKey, cache.PrivateCacheItem{
		Payload: response.Payload,
		Extra:   *extra,
	}, gocache.DefaultExpiration)

	return response.Payload, extra, nil
}

func (t *tesseraPrivateTxManager) Name() string {
	return "Tessera"
}

// don't serialize body if nil
func newOptionalJSONRequest(method string, path string, body interface{}) (*http.Request, error) {
	buf := new(bytes.Buffer)
	if body != nil {
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, err
		}
	}
	request, err := http.NewRequest(method, path, buf)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", fmt.Sprintf("quorum-v%s", params.QuorumVersion))
	request.Header.Set("Content-type", "application/json")
	request.Header.Set("Accept", "application/json")
	return request, nil
}
