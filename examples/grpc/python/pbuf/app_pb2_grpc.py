# Generated by the gRPC Python protocol compiler plugin. DO NOT EDIT!
import grpc

import app_pb2 as app__pb2


class BiobtreeServiceStub(object):
  # missing associated documentation comment in .proto file
  pass

  def __init__(self, channel):
    """Constructor.

    Args:
      channel: A grpc.Channel.
    """
    self.Get = channel.unary_unary(
        '/pbuf.BiobtreeService/Get',
        request_serializer=app__pb2.BiobtreeGetRequest.SerializeToString,
        response_deserializer=app__pb2.BiobtreeGetResponse.FromString,
        )
    self.GetPage = channel.unary_unary(
        '/pbuf.BiobtreeService/GetPage',
        request_serializer=app__pb2.BiobtreeGetPageRequest.SerializeToString,
        response_deserializer=app__pb2.BiobtreeGetPageResponse.FromString,
        )
    self.Filter = channel.unary_unary(
        '/pbuf.BiobtreeService/Filter',
        request_serializer=app__pb2.BiobtreeFilterRequest.SerializeToString,
        response_deserializer=app__pb2.BiobtreeFilterResponse.FromString,
        )
    self.Meta = channel.unary_unary(
        '/pbuf.BiobtreeService/Meta',
        request_serializer=app__pb2.BiobtreeMetaRequest.SerializeToString,
        response_deserializer=app__pb2.BiobtreeMetaResponse.FromString,
        )


class BiobtreeServiceServicer(object):
  # missing associated documentation comment in .proto file
  pass

  def Get(self, request, context):
    # missing associated documentation comment in .proto file
    pass
    context.set_code(grpc.StatusCode.UNIMPLEMENTED)
    context.set_details('Method not implemented!')
    raise NotImplementedError('Method not implemented!')

  def GetPage(self, request, context):
    # missing associated documentation comment in .proto file
    pass
    context.set_code(grpc.StatusCode.UNIMPLEMENTED)
    context.set_details('Method not implemented!')
    raise NotImplementedError('Method not implemented!')

  def Filter(self, request, context):
    # missing associated documentation comment in .proto file
    pass
    context.set_code(grpc.StatusCode.UNIMPLEMENTED)
    context.set_details('Method not implemented!')
    raise NotImplementedError('Method not implemented!')

  def Meta(self, request, context):
    # missing associated documentation comment in .proto file
    pass
    context.set_code(grpc.StatusCode.UNIMPLEMENTED)
    context.set_details('Method not implemented!')
    raise NotImplementedError('Method not implemented!')


def add_BiobtreeServiceServicer_to_server(servicer, server):
  rpc_method_handlers = {
      'Get': grpc.unary_unary_rpc_method_handler(
          servicer.Get,
          request_deserializer=app__pb2.BiobtreeGetRequest.FromString,
          response_serializer=app__pb2.BiobtreeGetResponse.SerializeToString,
      ),
      'GetPage': grpc.unary_unary_rpc_method_handler(
          servicer.GetPage,
          request_deserializer=app__pb2.BiobtreeGetPageRequest.FromString,
          response_serializer=app__pb2.BiobtreeGetPageResponse.SerializeToString,
      ),
      'Filter': grpc.unary_unary_rpc_method_handler(
          servicer.Filter,
          request_deserializer=app__pb2.BiobtreeFilterRequest.FromString,
          response_serializer=app__pb2.BiobtreeFilterResponse.SerializeToString,
      ),
      'Meta': grpc.unary_unary_rpc_method_handler(
          servicer.Meta,
          request_deserializer=app__pb2.BiobtreeMetaRequest.FromString,
          response_serializer=app__pb2.BiobtreeMetaResponse.SerializeToString,
      ),
  }
  generic_handler = grpc.method_handlers_generic_handler(
      'pbuf.BiobtreeService', rpc_method_handlers)
  server.add_generic_rpc_handlers((generic_handler,))
