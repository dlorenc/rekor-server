package pkg

import (
	"encoding/hex"

	"github.com/in-toto/in-toto-golang/in_toto"
	"github.com/projectrekor/rekor-server/logging"
)

func findArtifactHashes(link in_toto.Link) [][]byte {
	hashes := [][]byte{}
	for _, p := range link.Products {
		// Each product has to be a map[string]interface{}
		product, ok := p.(map[string]interface{})
		if !ok {
			// Not a product we can parse
			continue
		}

		for k, v := range product {
			// We have to look for products that declare a hash
			if k != "sha256" {
				continue
			}
			sha := v.(string)
			b, err := hex.DecodeString(sha)
			if err != nil {
				logging.Logger.Info("not a valid sha: %s", sha)
				break
			}
			hashes = append(hashes, b)
			break
		}
	}
	return hashes
}
