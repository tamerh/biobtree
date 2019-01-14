package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"./pbuf"
	"google.golang.org/grpc"
)

type biobtreegrpc struct {
	service service
}

func (g *biobtreegrpc) Start() {

	var port string
	if _, ok := appconf["grpcPort"]; ok {
		port = appconf["grpcPort"]
	} else {
		port = "7777"
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pbuf.RegisterBiobtreeServiceServer(grpcServer, g)
	// start the server
	go grpcServer.Serve(lis)

	/**
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
	**/
	log.Println("gRPC started at port->", port)

}
func (g *biobtreegrpc) Get(ctx context.Context, in *pbuf.BiobtreeGetRequest) (*pbuf.BiobtreeGetResponse, error) {

	res := g.service.search(in.Keywords)
	grpcRes := pbuf.BiobtreeGetResponse{}
	grpcRes.Results = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) GetPage(ctx context.Context, in *pbuf.BiobtreeGetPageRequest) (*pbuf.BiobtreeGetPageResponse, error) {

	res := g.service.page(in.Keyword, int(in.Dataset), int(in.Page), int(in.Total))
	grpcRes := pbuf.BiobtreeGetPageResponse{}
	grpcRes.Result = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) Filter(ctx context.Context, in *pbuf.BiobtreeFilterRequest) (*pbuf.BiobtreeFilterResponse, error) {

	res := g.service.filter(in.Keyword, uint32(in.Dataset), in.Filters, int(in.Page))
	grpcRes := pbuf.BiobtreeFilterResponse{}
	grpcRes.Result = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) Meta(ctx context.Context, in *pbuf.BiobtreeMetaRequest) (*pbuf.BiobtreeMetaResponse, error) {

	meta := pbuf.BiobtreeMetaResponse{}

	results := map[string]*pbuf.BiobtreeMetaKeyValue{}

	keymap := map[string]bool{}
	for k := range dataconf {
		id := dataconf[k]["id"]
		if _, ok := keymap[id]; !ok {

			keyvalues := map[string]string{}

			if len(dataconf[k]["name"]) > 0 {
				keyvalues["name"] = dataconf[k]["name"]
			} else {
				keyvalues["name"] = k
			}
			keyvalues["url"] = dataconf[k]["url"]

			metakeyvalue := pbuf.BiobtreeMetaKeyValue{}
			metakeyvalue.Keyvalues = keyvalues

			results[id] = &metakeyvalue

			keymap[id] = true
		}
	}
	meta.Results = results
	return &meta, nil

}
