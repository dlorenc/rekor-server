/*
Copyright © 2020 Luke Hinds <lhinds@redhat.com>

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
	"context"
	"fmt"
	"time"

	"github.com/projectrekor/rekor-server/logging"

	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
)

type trillianclient struct {
	lc     *client.LogClient
	client trillian.TrillianLogClient
	logID  int64
	ctx    context.Context
}

type Response struct {
	status   string
	leafhash string
}

func serverInstance(client trillian.TrillianLogClient, tLogID int64) *trillianclient {
	return &trillianclient{
		client: client,
		logID:  tLogID,
	}
}

func (s *trillianclient) root() (types.LogRootV1, error) {
	rqst := &trillian.GetLatestSignedLogRootRequest{
		LogId: s.logID,
	}
	resp, err := s.client.GetLatestSignedLogRoot(context.Background(), rqst)
	if err != nil {
		return types.LogRootV1{}, err
	}
	var root types.LogRootV1
	if err := root.UnmarshalBinary(resp.SignedLogRoot.LogRoot); err != nil {
		return types.LogRootV1{}, err
	}
	return root, nil
}

func (s *trillianclient) getInclusion(byteValue []byte, tLogID int64) (*Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	root, err := s.root()
	if err != nil {
		return &Response{}, err
	}

	logging.Logger.Info("Root hash: %x", root.RootHash)

	hasher := rfc6962.DefaultHasher
	leafHash := hasher.HashLeaf(byteValue)

	resp, err := s.client.GetInclusionProofByHash(ctx,
		&trillian.GetInclusionProofByHashRequest{
			LogId:    tLogID,
			LeafHash: leafHash,
			TreeSize: int64(root.TreeSize),
		})

	if err != nil {
		logging.Logger.Error(codes.Internal, "failed to get inclusion proof: %v", err)
		return &Response{}, nil
	}
	if err != nil {
		return &Response{}, err
	}
	if len(resp.Proof) < 1 {
		return &Response{}, nil
	}

	v := merkle.NewLogVerifier(rfc6962.DefaultHasher)

	for i, proof := range resp.Proof {
		hashes := proof.GetHashes()
		for j, hash := range hashes {
			logging.Logger.Infof("Proof[%d],hash[%d] == %x\n", i, j, hash)
		}
		if err := v.VerifyInclusionProof(proof.LeafIndex, int64(root.TreeSize), hashes, root.RootHash, leafHash); err != nil {
			return &Response{}, err
		}
	}

	return &Response{
		status: "OK",
	}, nil
}

func (s *trillianclient) addLeaf(byteValue []byte, tLogID int64) (*Response, error) {
	leaf := &trillian.LogLeaf{
		LeafValue: byteValue,
	}
	rqst := &trillian.QueueLeafRequest{
		LogId: tLogID,
		Leaf:  leaf,
	}
	resp, err := s.client.QueueLeaf(context.Background(), rqst)
	if err != nil {
		fmt.Println(err)
	}

	c := codes.Code(resp.QueuedLeaf.GetStatus().GetCode())
	if c != codes.OK && c != codes.AlreadyExists {
		logging.Logger.Error("Server Status: Bad status: %v", resp.QueuedLeaf.GetStatus())
	}
	if c == codes.OK {
		logging.Logger.Info("Server status: ok")
	} else if c == codes.AlreadyExists {
		logging.Logger.Info("Data already Exists")
	}

	return &Response{
		status: "OK",
	}, nil
}

func (s *trillianclient) getLeaf(byteValue []byte, tlog_id int64) (*Response, error) {
	hasher := rfc6962.DefaultHasher
	leafHash := hasher.HashLeaf(byteValue)

	rqst := &trillian.GetLeavesByHashRequest{
		LogId:    tlog_id,
		LeafHash: [][]byte{leafHash},
	}

	resp, err := s.client.GetLeavesByHash(context.Background(), rqst)
	if err != nil {
		logging.Logger.Fatal(err)
	}

	for i, logLeaf := range resp.GetLeaves() {
		leafValue := logLeaf.GetLeafValue()
		logging.Logger.Infof("trillianclient:get] %d: %s", i, string(leafValue))
	}

	return &Response{
		status: "OK",
	}, nil
}
