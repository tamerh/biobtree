package service

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"path/filepath"

	"biobtree/pbuf"
	"biobtree/query"

	"google.golang.org/grpc"
)

type biobtreegrpc struct {
	service *Service
}

func (g *biobtreegrpc) Start(prodMode bool) {

	var port string
	if prodMode {
		// Production mode: use prodGrpcPort config
		if _, ok := config.Appconf["prodGrpcPort"]; ok {
			port = config.Appconf["prodGrpcPort"]
		} else {
			log.Fatal("prodGrpcPort must be configured in application.param.json when using --prod flag")
		}
	} else {
		// Normal mode: use grpcPort config
		if _, ok := config.Appconf["grpcPort"]; ok {
			port = config.Appconf["grpcPort"]
		} else {
			log.Fatal("grpcPort must be configured in application.param.json")
		}
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

}
func (g *biobtreegrpc) Search(ctx context.Context, in *pbuf.SearchRequest) (*pbuf.SearchResponse, error) {

	if len(in.Terms) == 0 {
		return nil, fmt.Errorf("Input terms cannot be empty")
	}
	for _, term := range in.Terms {
		if len(term) == 0 {
			return nil, fmt.Errorf("Input term cannot be nil")
		}
	}
	grpcRes := pbuf.SearchResponse{}

	var filterq *query.Query
	if len(in.Query) > 0 {
		filterq = &query.Query{}
		filterq.Filter = in.Query
	}

	var src uint32
	var ok bool
	if len(in.Dataset) > 0 {

		src, ok = config.DataconfIDStringToInt[in.Dataset]
		if !ok {
			return nil, fmt.Errorf("Invalid dataset")
		}
	}

	// Check mode - lite or full (default)
	if in.Mode == "lite" {
		res, err := g.service.searchLite(in.Terms, src, in.Page, in.Dataset)
		if err != nil {
			return nil, err
		}
		grpcRes.ResultsLite = res
		return &grpcRes, nil
	}

	// Full mode (default)
	res, err := g.service.Search(in.Terms, src, in.Page, filterq, in.Detail, in.Url)

	if err != nil {
		return nil, err
	}
	grpcRes.Results = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) Mapping(ctx context.Context, in *pbuf.MappingRequest) (*pbuf.MappingResponse, error) {

	if len(in.Terms) == 0 {
		return nil, fmt.Errorf("Input terms cannot be empty")
	}
	for _, term := range in.Terms {
		if len(term) == 0 {
			return nil, fmt.Errorf("Input term cannot be nil")
		}
	}

	grpcRes := pbuf.MappingResponse{}

	// Check mode - lite or full (default)
	if in.Mode == "lite" {
		res, err := g.service.MapFilterLite(in.Terms, in.Query, in.Page)
		if err != nil {
			return nil, err
		}
		grpcRes.ResultsLite = res
		return &grpcRes, nil
	}

	// Full mode (default)
	res, err := g.service.MapFilter(in.Terms, in.Query, in.Page)
	if err != nil {
		return nil, err
	}
	grpcRes.Results = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) Entry(ctx context.Context, in *pbuf.EntryRequest) (*pbuf.EntryResponse, error) {

	if len(in.Identifier) == 0 {
		return nil, fmt.Errorf("identifier cannot be empty")
	}

	if len(in.Dataset) == 0 {
		return nil, fmt.Errorf("dataset cannot be empty")
	}

	grpcRes := pbuf.EntryResponse{}

	var src uint32
	var ok bool
	if len(in.Dataset) > 0 {

		src, ok = config.DataconfIDStringToInt[in.Dataset]
		if !ok {
			return nil, fmt.Errorf("Invalid dataset")
		}
	}
	res, err := g.service.LookupByDataset(in.Identifier, src)
	if err != nil {
		return nil, err
	}
	grpcRes.Result = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) Meta(ctx context.Context, in *pbuf.MetaRequest) (*pbuf.MetaResponse, error) {

	return g.service.meta(), nil

}

func (g *biobtreegrpc) ListGenomes(ctx context.Context, in *pbuf.ListGenomesRequest) (*pbuf.ListGenomesResponse, error) {

	switch in.Type {
	case "ensembl", "ensembl_bacteria", "ensembl_fungi", "ensembl_metazoa", "ensembl_plants", "ensembl_protists":

		content, err := ioutil.ReadFile(filepath.FromSlash("ensembl/" + in.Type + ".paths.json"))
		if err != nil {
			return nil, err
		}
		grpcRes := pbuf.ListGenomesResponse{}
		grpcRes.Results = string(content)
		return &grpcRes, nil
	default:
		return nil, fmt.Errorf("Invalid genome type valid types are 'ensembl', 'ensembl_bacteria', 'ensembl_fungi', 'ensembl_metazoa', 'ensembl_plants', 'ensembl_protists'")
	}

}
