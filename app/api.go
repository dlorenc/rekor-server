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

// https://pace.dev/blog/2018/05/09/how-I-write-http-services-after-eight-years.html
// https://github.com/dhax/go-base/blob/master/api/api.go
// curl http://localhost:3000/add -F "fileupload=@/tmp/file" -vvv

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/google/trillian"
	"github.com/projectrekor/rekor-server/logging"
	"github.com/projectrekor/rekor-server/pkg"
)

type API struct {
	clients *pkg.Clients
}

func NewAPI() (*API, error) {
	clients, err := pkg.NewClients()
	if err != nil {
		return nil, err
	}

	return &API{
		clients: clients,
	}, nil
}

func (api *API) ping(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong!")
}

func (api *API) getHandler(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("fileupload")

	if err != nil {
		writeError(w, err)
		return
	}
	defer file.Close()

	// return that we have successfully uploaded our file!
	fmt.Fprintf(w, "Successfully Uploaded File\n")
	logging.Logger.Info("Received file : ", header.Filename)

	byteLeaf, err := ioutil.ReadAll(file)
	if err != nil {
		writeError(w, err)
		return
	}

	server := serverInstance(api.clients.LogClient, api.clients.TLogID)
	resp, err := server.getLeaf(byteLeaf, api.clients.TLogID)
	if err != nil {
		writeError(w, err)
		return
	}
	logging.Logger.Infof("Server PUT Response: %s", resp.status)
	fmt.Fprintf(w, "Server PUT Response: %s\n", resp.status)
}

func (api *API) lookupHandler(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	hashBytes, err := hex.DecodeString(hash)
	if err != nil {
		writeError(w, err)
		return
	}

	response, err := api.clients.MapClient.GetLeaf(r.Context(), &trillian.GetMapLeafRequest{
		MapId: api.clients.TMapID,
		Index: hashBytes,
	})

	logIndex, err := strconv.ParseInt(string(response.MapLeafInclusion.Leaf.LeafValue), 10, 64)
	if err != nil {
		writeError(w, err)
		return
	}
	// Lookup the value from the log now at that index.
	resp, err := api.clients.LogClient.GetLeavesByIndex(r.Context(), &trillian.GetLeavesByIndexRequest{
		LogId:     api.clients.TLogID,
		LeafIndex: []int64{logIndex},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	for _, l := range resp.Leaves {
		fmt.Fprintf(w, "----\n%s\n", string(l.LeafValue))
	}
	return
}

func (api *API) addHandler(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("fileupload")

	if err != nil {
		writeError(w, err)
		return
	}
	defer file.Close()

	// return that we have successfully uploaded our file!
	fmt.Fprintf(w, "Successfully Uploaded File\n")
	logging.Logger.Info("Received file : ", header.Filename)

	byteLeaf, err := ioutil.ReadAll(file)
	if err != nil {
		writeError(w, err)
		return
	}

	server := serverInstance(api.clients.LogClient, api.clients.TLogID)

	resp, err := server.addLeaf(byteLeaf, api.clients.TLogID)
	if err != nil {
		writeError(w, err)
		return
	}
	logging.Logger.Infof("Server PUT Response: %s", resp.status)
	fmt.Fprintf(w, "Server PUT Response: %s", resp.status)
}

func New() (*chi.Mux, error) {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	api, err := NewAPI()
	if err != nil {
		return nil, err
	}
	router.Post("/api/v1/add", api.addHandler)
	router.Post("/api/v1/get", api.getHandler)
	router.Get("/api/v1//ping", api.ping)
	router.Get("/api/v1/lookup", api.lookupHandler)
	return router, nil
}

func writeError(w http.ResponseWriter, err error) {
	logging.Logger.Error(err)
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "Server error: %v\n", err)
}
