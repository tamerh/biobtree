package service

import (
	"context"
	"fmt"
	"log"
	"net"

	"biobtree/pbuf"

	"google.golang.org/grpc"
)

type biobtreegrpc struct {
	service service
}

func (g *biobtreegrpc) Start() {

	var port string
	if _, ok := config.Appconf["grpcPort"]; ok {
		port = config.Appconf["grpcPort"]
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

	/**res, err := g.service.search(in.Keywords)
	if err != nil {
		return nil, err
	}**/
	grpcRes := pbuf.BiobtreeGetResponse{}
	//TODO grpcRes.Results = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) MapFilter(ctx context.Context, in *pbuf.BiobtreeMapFilterRequest) (*pbuf.BiobtreeMapFilterResponse, error) {

	//res := g.service.search(in.Keywords)
	// TODO
	grpcRes := pbuf.BiobtreeMapFilterResponse{}
	grpcRes.Results = nil
	return &grpcRes, nil

}

func (g *biobtreegrpc) GetPage(ctx context.Context, in *pbuf.BiobtreeGetPageRequest) (*pbuf.BiobtreeGetPageResponse, error) {

	res, err := g.service.page(in.Keyword, int(in.Dataset), int(in.Page), int(in.Total))
	if err != nil {
		return nil, err
	}
	grpcRes := pbuf.BiobtreeGetPageResponse{}
	grpcRes.Result = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) Filter(ctx context.Context, in *pbuf.BiobtreeFilterRequest) (*pbuf.BiobtreeFilterResponse, error) {

	res, err := g.service.filter(in.Keyword, uint32(in.Dataset), in.Filters, int(in.Page))
	if err != nil {
		return nil, err
	}
	grpcRes := pbuf.BiobtreeFilterResponse{}
	grpcRes.Result = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) Meta(ctx context.Context, in *pbuf.BiobtreeMetaRequest) (*pbuf.BiobtreeMetaResponse, error) {

	meta := pbuf.BiobtreeMetaResponse{}

	results := map[string]*pbuf.BiobtreeMetaKeyValue{}

	keymap := map[string]bool{}
	for k := range config.Dataconf {
		id := config.Dataconf[k]["id"]
		if _, ok := keymap[id]; !ok {

			keyvalues := map[string]string{}

			if len(config.Dataconf[k]["name"]) > 0 {
				keyvalues["name"] = config.Dataconf[k]["name"]
			} else {
				keyvalues["name"] = k
			}
			keyvalues["url"] = config.Dataconf[k]["url"]

			metakeyvalue := pbuf.BiobtreeMetaKeyValue{}
			metakeyvalue.Keyvalues = keyvalues

			results[id] = &metakeyvalue

			keymap[id] = true
		}
	}
	meta.Results = results
	return &meta, nil

}
