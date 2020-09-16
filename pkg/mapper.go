package pkg

import (
	"context"
	gocrypto "crypto"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/in-toto/in-toto-golang/in_toto"
	"github.com/projectrekor/rekor-server/logging"
)

func StartMapper() {
	clients, err := NewClients()
	if err != nil {
		logging.Logger.Fatal(err)
	}

	ctx := context.Background()

	go func() {
		for {
			if err := RunMapper(ctx, clients); err != nil {
				logging.Logger.Error(err)
			}
			time.Sleep(10 * time.Second)
		}
	}()
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	sig := <-quit
	logging.Logger.Info("Shutting down mapper... Reason:", sig)
}

type MapperState struct {
	LastLeaf int64
}

func RunMapper(ctx context.Context, clients *Clients) error {
	// Get the last known good leaf state
	revision, lc, state, err := getLastMapState(ctx, clients)
	if err != nil {
		return err
	}
	if state.LastLeaf >= lc-1 {
		logging.Logger.Info("No new leaves.")
		return nil
	}

	nextLeafToGet := state.LastLeaf + 1
	// Get a leaf from the log, process it into the map, and update the map metadata at the same time
	resp, err := clients.LogClient.GetLeavesByIndex(ctx, &trillian.GetLeavesByIndexRequest{
		LogId:     clients.TLogID,
		LeafIndex: []int64{nextLeafToGet},
	})
	if err != nil {
		return err
	}

	nextRevision := revision + 1
	// There should be only one leaf to process here, but it may have multiple products.
	for _, leaf := range resp.Leaves {
		logging.Logger.Infof("Processing leaf %d", leaf.LeafIndex)
		var link in_toto.Link
		if err := json.Unmarshal(leaf.LeafValue, &link); err != nil {
			logging.Logger.Warnf("Leaf index %d is not a link file. %v", leaf.LeafIndex, err)
			continue
		}

		logging.Logger.Infof("Examining %s", string(leaf.LeafValue))
		artifacts := findArtifactHashes(link)
		leaves := []*trillian.MapLeaf{}
		for _, a := range artifacts {
			val := strconv.FormatInt(leaf.LeafIndex, 10)
			logging.Logger.Infof("Found hash %s on leaf %d", hex.EncodeToString(a), leaf.LeafIndex)
			logging.Logger.Infof("Setting index %s to value %s", string(a), val)
			leaves = append(leaves, &trillian.MapLeaf{
				Index:     a,
				LeafValue: []byte(val),
			})
		}
		stateBytes, err := json.Marshal(MapperState{
			LastLeaf: leaf.LeafIndex,
		})
		if err != nil {
			return err
		}
		if _, err := clients.MapClient.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
			MapId:    clients.TMapID,
			Leaves:   leaves,
			Metadata: stateBytes,
			Revision: int64(nextRevision),
		}); err != nil {
			return err
		}
		nextRevision++
	}
	return nil
}

func getLastMapState(ctx context.Context, clients *Clients) (revision uint64, leafCount int64, state MapperState, err error) {
	signedMapRoot, err := clients.MapClient.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{
		MapId: clients.TMapID,
	})
	if err != nil {
		return 0, 0, state, err
	}

	mrv1, err := crypto.VerifySignedMapRoot(clients.MapPubKey, gocrypto.SHA256, signedMapRoot.MapRoot)
	if err != nil {
		return 0, 0, state, err
	}
	if err := json.Unmarshal(mrv1.Metadata, &state); err != nil {
		logging.Logger.Error(err)
	}

	lc, err := clients.LogClient.GetSequencedLeafCount(ctx, &trillian.GetSequencedLeafCountRequest{
		LogId: clients.TLogID,
	})
	if err != nil {
		return 0, 0, state, err
	}
	return mrv1.Revision, lc.LeafCount, state, nil
}

func getKey(ctx context.Context, adminClient trillian.TrillianAdminClient, logID int64) (interface{}, error) {
	tree, err := adminClient.GetTree(ctx, &trillian.GetTreeRequest{TreeId: logID})
	if err != nil {
		return nil, fmt.Errorf("call to GetTree failed: %v", err)
	}

	if tree == nil {
		return nil, fmt.Errorf("log %d not found", logID)
	}

	publicKey := tree.GetPublicKey()
	return x509.ParsePKIXPublicKey(publicKey.GetDer())
}
