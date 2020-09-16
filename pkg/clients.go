package pkg

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/projectrekor/rekor-server/logging"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

type Clients struct {
	TLogID         int64
	TMapID         int64
	LogAdminClient trillian.TrillianAdminClient
	LogClient      trillian.TrillianLogClient
	MapAdminClient trillian.TrillianAdminClient
	MapClient      trillian.TrillianMapClient
	MapPubKey      interface{}
}

func dial(ctx context.Context, rpcserver string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Set up and test connection to rpc server
	conn, err := grpc.DialContext(ctx, rpcserver, grpc.WithInsecure())
	if err != nil {
		logging.Logger.Fatalf("Failed to connect to log server:", err)
	}
	return conn, nil
}

func NewClients() (*Clients, error) {
	logRpcServer := fmt.Sprintf("%s:%d",
		viper.GetString("trillian_log_server.address"),
		viper.GetInt("trillian_log_server.port"))
	ctx := context.Background()
	tConn, err := dial(ctx, logRpcServer)
	if err != nil {
		return nil, err
	}
	logAdminClient := trillian.NewTrillianAdminClient(tConn)
	logClient := trillian.NewTrillianLogClient(tConn)

	tLogID := viper.GetInt64("trillian_log_server.tlog_id")
	if tLogID == 0 {
		t, err := createAndInitTree(ctx, logAdminClient, logClient)
		if err != nil {
			return nil, err
		}
		tLogID = t.TreeId
	}

	mapRpcServer := fmt.Sprintf("%s:%d",
		viper.GetString("trillian_map_server.address"),
		viper.GetInt("trillian_map_server.port"))
	mConn, err := dial(ctx, mapRpcServer)
	if err != nil {
		return nil, err
	}
	mapAdminClient := trillian.NewTrillianAdminClient(mConn)
	mapClient := trillian.NewTrillianMapClient(mConn)
	tMapID := viper.GetInt64("trillian_map_server.tmap_id")
	if tMapID == 0 {
		t, err := createAndInitMap(ctx, mapAdminClient, mapClient)
		if err != nil {
			return nil, err
		}
		tMapID = t.TreeId
	}
	mapPubKey, err := getKey(ctx, mapAdminClient, tMapID)
	if err != nil {
		logging.Logger.Fatal(err)
	}

	return &Clients{
		TLogID:         tLogID,
		TMapID:         tMapID,
		LogAdminClient: logAdminClient,
		LogClient:      logClient,
		MapAdminClient: mapAdminClient,
		MapClient:      mapClient,
		MapPubKey:      mapPubKey,
	}, nil
}

func createAndInitTree(ctx context.Context, adminClient trillian.TrillianAdminClient, logClient trillian.TrillianLogClient) (*trillian.Tree, error) {
	// First look for and use an existing tree
	trees, err := adminClient.ListTrees(ctx, &trillian.ListTreesRequest{})
	if err != nil {
		return nil, err
	}

	for _, t := range trees.Tree {
		if t.TreeType == trillian.TreeType_LOG {
			return t, nil
		}
	}

	// Otherwise create and initialize one
	t, err := adminClient.CreateTree(ctx, &trillian.CreateTreeRequest{
		Tree: &trillian.Tree{
			TreeType:           trillian.TreeType_LOG,
			HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
			HashAlgorithm:      sigpb.DigitallySigned_SHA256,
			SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			TreeState:          trillian.TreeState_ACTIVE,
			MaxRootDuration:    ptypes.DurationProto(time.Hour),
		},
		KeySpec: &keyspb.Specification{
			Params: &keyspb.Specification_EcdsaParams{
				EcdsaParams: &keyspb.Specification_ECDSA{},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if err := client.InitLog(ctx, t, logClient); err != nil {
		return nil, err
	}
	return t, nil
}

func createAndInitMap(ctx context.Context, adminClient trillian.TrillianAdminClient, mapClient trillian.TrillianMapClient) (*trillian.Tree, error) {
	// First look for and use an existing tree
	trees, err := adminClient.ListTrees(ctx, &trillian.ListTreesRequest{})
	if err != nil {
		return nil, err
	}

	for _, t := range trees.Tree {
		if t.TreeType == trillian.TreeType_MAP {
			return t, nil
		}
	}

	// Otherwise create and initialize one
	t, err := adminClient.CreateTree(ctx, &trillian.CreateTreeRequest{
		Tree: &trillian.Tree{
			TreeType:           trillian.TreeType_MAP,
			HashStrategy:       trillian.HashStrategy_CONIKS_SHA256,
			HashAlgorithm:      sigpb.DigitallySigned_SHA256,
			SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			TreeState:          trillian.TreeState_ACTIVE,
			MaxRootDuration:    ptypes.DurationProto(time.Hour),
		},
		KeySpec: &keyspb.Specification{
			Params: &keyspb.Specification_EcdsaParams{
				EcdsaParams: &keyspb.Specification_ECDSA{},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if err := client.InitMap(ctx, t, mapClient); err != nil {
		return nil, err
	}
	return t, nil
}
