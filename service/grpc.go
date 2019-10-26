package service

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"path/filepath"
	"strings"

	"biobtree/pbuf"
	"biobtree/query"

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

	res, err := g.service.search(in.Terms, src, in.Page, filterq, in.Detail, in.Url)

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

	var src uint32
	var ok bool
	if len(in.Dataset) > 0 {

		src, ok = config.DataconfIDStringToInt[in.Dataset]
		if !ok {
			return nil, fmt.Errorf("Invalid dataset")
		}
	}

	grpcRes := pbuf.MappingResponse{}
	res, err := g.service.mapFilter(in.Terms, src, in.Query, in.Page)
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
	res, err := g.service.getLmdbResult2(strings.ToUpper(in.Identifier), src)
	if err != nil {
		return nil, err
	}
	g.service.setURL(res)
	grpcRes.Result = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) Page(ctx context.Context, in *pbuf.PageRequest) (*pbuf.PageResponse, error) {

	if len(in.Identifier) == 0 {
		return nil, fmt.Errorf("Input identifier cannot be empty")
	}

	var src uint32
	var ok bool
	if len(in.Dataset) > 0 {

		src, ok = config.DataconfIDStringToInt[in.Dataset]
		if !ok {
			return nil, fmt.Errorf("Invalid dataset")
		}
	}

	res, err := g.service.page(in.Identifier, int(src), int(in.Page), int(in.Total))
	if err != nil {
		return nil, err
	}
	grpcRes := pbuf.PageResponse{}
	grpcRes.Result = res
	return &grpcRes, nil

}

func (g *biobtreegrpc) Filter(ctx context.Context, in *pbuf.FilterRequest) (*pbuf.FilterResponse, error) {

	if len(in.Identifier) == 0 {
		return nil, fmt.Errorf("Input identifier cannot be empty")
	}

	if len(in.Filters) == 0 {
		return nil, fmt.Errorf("Filters cannot be empty")
	}

	if len(in.Dataset) == 0 {
		return nil, fmt.Errorf("Dataset cannot be empty")
	}

	var src uint32
	var ok bool
	if len(in.Dataset) > 0 {

		src, ok = config.DataconfIDStringToInt[in.Dataset]
		if !ok {
			return nil, fmt.Errorf("Invalid dataset")
		}
	}

	var filters []uint32
	for _, filterstr := range in.Filters {

		filtersrc, ok := config.DataconfIDStringToInt[filterstr]
		if !ok {
			return nil, fmt.Errorf("Invalid filter dataset")
		}
		filters = append(filters, uint32(filtersrc))

	}

	res, err := g.service.filter(in.Identifier, src, filters, int(in.Page))
	if err != nil {
		return nil, err
	}
	grpcRes := pbuf.FilterResponse{}
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
