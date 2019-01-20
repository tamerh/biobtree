package pbuf;

import static io.grpc.MethodDescriptor.generateFullMethodName;
import static io.grpc.stub.ClientCalls.asyncBidiStreamingCall;
import static io.grpc.stub.ClientCalls.asyncClientStreamingCall;
import static io.grpc.stub.ClientCalls.asyncServerStreamingCall;
import static io.grpc.stub.ClientCalls.asyncUnaryCall;
import static io.grpc.stub.ClientCalls.blockingServerStreamingCall;
import static io.grpc.stub.ClientCalls.blockingUnaryCall;
import static io.grpc.stub.ClientCalls.futureUnaryCall;
import static io.grpc.stub.ServerCalls.asyncBidiStreamingCall;
import static io.grpc.stub.ServerCalls.asyncClientStreamingCall;
import static io.grpc.stub.ServerCalls.asyncServerStreamingCall;
import static io.grpc.stub.ServerCalls.asyncUnaryCall;
import static io.grpc.stub.ServerCalls.asyncUnimplementedStreamingCall;
import static io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall;

/**
 */
@javax.annotation.Generated(
    value = "by gRPC proto compiler (version 1.18.0)",
    comments = "Source: app.proto")
public final class BiobtreeServiceGrpc {

  private BiobtreeServiceGrpc() {}

  public static final String SERVICE_NAME = "pbuf.BiobtreeService";

  // Static method descriptors that strictly reflect the proto.
  private static volatile io.grpc.MethodDescriptor<pbuf.App.BiobtreeGetRequest,
      pbuf.App.BiobtreeGetResponse> getGetMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "Get",
      requestType = pbuf.App.BiobtreeGetRequest.class,
      responseType = pbuf.App.BiobtreeGetResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<pbuf.App.BiobtreeGetRequest,
      pbuf.App.BiobtreeGetResponse> getGetMethod() {
    io.grpc.MethodDescriptor<pbuf.App.BiobtreeGetRequest, pbuf.App.BiobtreeGetResponse> getGetMethod;
    if ((getGetMethod = BiobtreeServiceGrpc.getGetMethod) == null) {
      synchronized (BiobtreeServiceGrpc.class) {
        if ((getGetMethod = BiobtreeServiceGrpc.getGetMethod) == null) {
          BiobtreeServiceGrpc.getGetMethod = getGetMethod = 
              io.grpc.MethodDescriptor.<pbuf.App.BiobtreeGetRequest, pbuf.App.BiobtreeGetResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(
                  "pbuf.BiobtreeService", "Get"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  pbuf.App.BiobtreeGetRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  pbuf.App.BiobtreeGetResponse.getDefaultInstance()))
                  .setSchemaDescriptor(new BiobtreeServiceMethodDescriptorSupplier("Get"))
                  .build();
          }
        }
     }
     return getGetMethod;
  }

  private static volatile io.grpc.MethodDescriptor<pbuf.App.BiobtreeGetPageRequest,
      pbuf.App.BiobtreeGetPageResponse> getGetPageMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "GetPage",
      requestType = pbuf.App.BiobtreeGetPageRequest.class,
      responseType = pbuf.App.BiobtreeGetPageResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<pbuf.App.BiobtreeGetPageRequest,
      pbuf.App.BiobtreeGetPageResponse> getGetPageMethod() {
    io.grpc.MethodDescriptor<pbuf.App.BiobtreeGetPageRequest, pbuf.App.BiobtreeGetPageResponse> getGetPageMethod;
    if ((getGetPageMethod = BiobtreeServiceGrpc.getGetPageMethod) == null) {
      synchronized (BiobtreeServiceGrpc.class) {
        if ((getGetPageMethod = BiobtreeServiceGrpc.getGetPageMethod) == null) {
          BiobtreeServiceGrpc.getGetPageMethod = getGetPageMethod = 
              io.grpc.MethodDescriptor.<pbuf.App.BiobtreeGetPageRequest, pbuf.App.BiobtreeGetPageResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(
                  "pbuf.BiobtreeService", "GetPage"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  pbuf.App.BiobtreeGetPageRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  pbuf.App.BiobtreeGetPageResponse.getDefaultInstance()))
                  .setSchemaDescriptor(new BiobtreeServiceMethodDescriptorSupplier("GetPage"))
                  .build();
          }
        }
     }
     return getGetPageMethod;
  }

  private static volatile io.grpc.MethodDescriptor<pbuf.App.BiobtreeFilterRequest,
      pbuf.App.BiobtreeFilterResponse> getFilterMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "Filter",
      requestType = pbuf.App.BiobtreeFilterRequest.class,
      responseType = pbuf.App.BiobtreeFilterResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<pbuf.App.BiobtreeFilterRequest,
      pbuf.App.BiobtreeFilterResponse> getFilterMethod() {
    io.grpc.MethodDescriptor<pbuf.App.BiobtreeFilterRequest, pbuf.App.BiobtreeFilterResponse> getFilterMethod;
    if ((getFilterMethod = BiobtreeServiceGrpc.getFilterMethod) == null) {
      synchronized (BiobtreeServiceGrpc.class) {
        if ((getFilterMethod = BiobtreeServiceGrpc.getFilterMethod) == null) {
          BiobtreeServiceGrpc.getFilterMethod = getFilterMethod = 
              io.grpc.MethodDescriptor.<pbuf.App.BiobtreeFilterRequest, pbuf.App.BiobtreeFilterResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(
                  "pbuf.BiobtreeService", "Filter"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  pbuf.App.BiobtreeFilterRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  pbuf.App.BiobtreeFilterResponse.getDefaultInstance()))
                  .setSchemaDescriptor(new BiobtreeServiceMethodDescriptorSupplier("Filter"))
                  .build();
          }
        }
     }
     return getFilterMethod;
  }

  private static volatile io.grpc.MethodDescriptor<pbuf.App.BiobtreeMetaRequest,
      pbuf.App.BiobtreeMetaResponse> getMetaMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "Meta",
      requestType = pbuf.App.BiobtreeMetaRequest.class,
      responseType = pbuf.App.BiobtreeMetaResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<pbuf.App.BiobtreeMetaRequest,
      pbuf.App.BiobtreeMetaResponse> getMetaMethod() {
    io.grpc.MethodDescriptor<pbuf.App.BiobtreeMetaRequest, pbuf.App.BiobtreeMetaResponse> getMetaMethod;
    if ((getMetaMethod = BiobtreeServiceGrpc.getMetaMethod) == null) {
      synchronized (BiobtreeServiceGrpc.class) {
        if ((getMetaMethod = BiobtreeServiceGrpc.getMetaMethod) == null) {
          BiobtreeServiceGrpc.getMetaMethod = getMetaMethod = 
              io.grpc.MethodDescriptor.<pbuf.App.BiobtreeMetaRequest, pbuf.App.BiobtreeMetaResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(
                  "pbuf.BiobtreeService", "Meta"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  pbuf.App.BiobtreeMetaRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  pbuf.App.BiobtreeMetaResponse.getDefaultInstance()))
                  .setSchemaDescriptor(new BiobtreeServiceMethodDescriptorSupplier("Meta"))
                  .build();
          }
        }
     }
     return getMetaMethod;
  }

  /**
   * Creates a new async stub that supports all call types for the service
   */
  public static BiobtreeServiceStub newStub(io.grpc.Channel channel) {
    return new BiobtreeServiceStub(channel);
  }

  /**
   * Creates a new blocking-style stub that supports unary and streaming output calls on the service
   */
  public static BiobtreeServiceBlockingStub newBlockingStub(
      io.grpc.Channel channel) {
    return new BiobtreeServiceBlockingStub(channel);
  }

  /**
   * Creates a new ListenableFuture-style stub that supports unary calls on the service
   */
  public static BiobtreeServiceFutureStub newFutureStub(
      io.grpc.Channel channel) {
    return new BiobtreeServiceFutureStub(channel);
  }

  /**
   */
  public static abstract class BiobtreeServiceImplBase implements io.grpc.BindableService {

    /**
     */
    public void get(pbuf.App.BiobtreeGetRequest request,
        io.grpc.stub.StreamObserver<pbuf.App.BiobtreeGetResponse> responseObserver) {
      asyncUnimplementedUnaryCall(getGetMethod(), responseObserver);
    }

    /**
     */
    public void getPage(pbuf.App.BiobtreeGetPageRequest request,
        io.grpc.stub.StreamObserver<pbuf.App.BiobtreeGetPageResponse> responseObserver) {
      asyncUnimplementedUnaryCall(getGetPageMethod(), responseObserver);
    }

    /**
     */
    public void filter(pbuf.App.BiobtreeFilterRequest request,
        io.grpc.stub.StreamObserver<pbuf.App.BiobtreeFilterResponse> responseObserver) {
      asyncUnimplementedUnaryCall(getFilterMethod(), responseObserver);
    }

    /**
     */
    public void meta(pbuf.App.BiobtreeMetaRequest request,
        io.grpc.stub.StreamObserver<pbuf.App.BiobtreeMetaResponse> responseObserver) {
      asyncUnimplementedUnaryCall(getMetaMethod(), responseObserver);
    }

    @java.lang.Override public final io.grpc.ServerServiceDefinition bindService() {
      return io.grpc.ServerServiceDefinition.builder(getServiceDescriptor())
          .addMethod(
            getGetMethod(),
            asyncUnaryCall(
              new MethodHandlers<
                pbuf.App.BiobtreeGetRequest,
                pbuf.App.BiobtreeGetResponse>(
                  this, METHODID_GET)))
          .addMethod(
            getGetPageMethod(),
            asyncUnaryCall(
              new MethodHandlers<
                pbuf.App.BiobtreeGetPageRequest,
                pbuf.App.BiobtreeGetPageResponse>(
                  this, METHODID_GET_PAGE)))
          .addMethod(
            getFilterMethod(),
            asyncUnaryCall(
              new MethodHandlers<
                pbuf.App.BiobtreeFilterRequest,
                pbuf.App.BiobtreeFilterResponse>(
                  this, METHODID_FILTER)))
          .addMethod(
            getMetaMethod(),
            asyncUnaryCall(
              new MethodHandlers<
                pbuf.App.BiobtreeMetaRequest,
                pbuf.App.BiobtreeMetaResponse>(
                  this, METHODID_META)))
          .build();
    }
  }

  /**
   */
  public static final class BiobtreeServiceStub extends io.grpc.stub.AbstractStub<BiobtreeServiceStub> {
    private BiobtreeServiceStub(io.grpc.Channel channel) {
      super(channel);
    }

    private BiobtreeServiceStub(io.grpc.Channel channel,
        io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected BiobtreeServiceStub build(io.grpc.Channel channel,
        io.grpc.CallOptions callOptions) {
      return new BiobtreeServiceStub(channel, callOptions);
    }

    /**
     */
    public void get(pbuf.App.BiobtreeGetRequest request,
        io.grpc.stub.StreamObserver<pbuf.App.BiobtreeGetResponse> responseObserver) {
      asyncUnaryCall(
          getChannel().newCall(getGetMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void getPage(pbuf.App.BiobtreeGetPageRequest request,
        io.grpc.stub.StreamObserver<pbuf.App.BiobtreeGetPageResponse> responseObserver) {
      asyncUnaryCall(
          getChannel().newCall(getGetPageMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void filter(pbuf.App.BiobtreeFilterRequest request,
        io.grpc.stub.StreamObserver<pbuf.App.BiobtreeFilterResponse> responseObserver) {
      asyncUnaryCall(
          getChannel().newCall(getFilterMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void meta(pbuf.App.BiobtreeMetaRequest request,
        io.grpc.stub.StreamObserver<pbuf.App.BiobtreeMetaResponse> responseObserver) {
      asyncUnaryCall(
          getChannel().newCall(getMetaMethod(), getCallOptions()), request, responseObserver);
    }
  }

  /**
   */
  public static final class BiobtreeServiceBlockingStub extends io.grpc.stub.AbstractStub<BiobtreeServiceBlockingStub> {
    private BiobtreeServiceBlockingStub(io.grpc.Channel channel) {
      super(channel);
    }

    private BiobtreeServiceBlockingStub(io.grpc.Channel channel,
        io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected BiobtreeServiceBlockingStub build(io.grpc.Channel channel,
        io.grpc.CallOptions callOptions) {
      return new BiobtreeServiceBlockingStub(channel, callOptions);
    }

    /**
     */
    public pbuf.App.BiobtreeGetResponse get(pbuf.App.BiobtreeGetRequest request) {
      return blockingUnaryCall(
          getChannel(), getGetMethod(), getCallOptions(), request);
    }

    /**
     */
    public pbuf.App.BiobtreeGetPageResponse getPage(pbuf.App.BiobtreeGetPageRequest request) {
      return blockingUnaryCall(
          getChannel(), getGetPageMethod(), getCallOptions(), request);
    }

    /**
     */
    public pbuf.App.BiobtreeFilterResponse filter(pbuf.App.BiobtreeFilterRequest request) {
      return blockingUnaryCall(
          getChannel(), getFilterMethod(), getCallOptions(), request);
    }

    /**
     */
    public pbuf.App.BiobtreeMetaResponse meta(pbuf.App.BiobtreeMetaRequest request) {
      return blockingUnaryCall(
          getChannel(), getMetaMethod(), getCallOptions(), request);
    }
  }

  /**
   */
  public static final class BiobtreeServiceFutureStub extends io.grpc.stub.AbstractStub<BiobtreeServiceFutureStub> {
    private BiobtreeServiceFutureStub(io.grpc.Channel channel) {
      super(channel);
    }

    private BiobtreeServiceFutureStub(io.grpc.Channel channel,
        io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected BiobtreeServiceFutureStub build(io.grpc.Channel channel,
        io.grpc.CallOptions callOptions) {
      return new BiobtreeServiceFutureStub(channel, callOptions);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<pbuf.App.BiobtreeGetResponse> get(
        pbuf.App.BiobtreeGetRequest request) {
      return futureUnaryCall(
          getChannel().newCall(getGetMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<pbuf.App.BiobtreeGetPageResponse> getPage(
        pbuf.App.BiobtreeGetPageRequest request) {
      return futureUnaryCall(
          getChannel().newCall(getGetPageMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<pbuf.App.BiobtreeFilterResponse> filter(
        pbuf.App.BiobtreeFilterRequest request) {
      return futureUnaryCall(
          getChannel().newCall(getFilterMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<pbuf.App.BiobtreeMetaResponse> meta(
        pbuf.App.BiobtreeMetaRequest request) {
      return futureUnaryCall(
          getChannel().newCall(getMetaMethod(), getCallOptions()), request);
    }
  }

  private static final int METHODID_GET = 0;
  private static final int METHODID_GET_PAGE = 1;
  private static final int METHODID_FILTER = 2;
  private static final int METHODID_META = 3;

  private static final class MethodHandlers<Req, Resp> implements
      io.grpc.stub.ServerCalls.UnaryMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.ServerStreamingMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.ClientStreamingMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.BidiStreamingMethod<Req, Resp> {
    private final BiobtreeServiceImplBase serviceImpl;
    private final int methodId;

    MethodHandlers(BiobtreeServiceImplBase serviceImpl, int methodId) {
      this.serviceImpl = serviceImpl;
      this.methodId = methodId;
    }

    @java.lang.Override
    @java.lang.SuppressWarnings("unchecked")
    public void invoke(Req request, io.grpc.stub.StreamObserver<Resp> responseObserver) {
      switch (methodId) {
        case METHODID_GET:
          serviceImpl.get((pbuf.App.BiobtreeGetRequest) request,
              (io.grpc.stub.StreamObserver<pbuf.App.BiobtreeGetResponse>) responseObserver);
          break;
        case METHODID_GET_PAGE:
          serviceImpl.getPage((pbuf.App.BiobtreeGetPageRequest) request,
              (io.grpc.stub.StreamObserver<pbuf.App.BiobtreeGetPageResponse>) responseObserver);
          break;
        case METHODID_FILTER:
          serviceImpl.filter((pbuf.App.BiobtreeFilterRequest) request,
              (io.grpc.stub.StreamObserver<pbuf.App.BiobtreeFilterResponse>) responseObserver);
          break;
        case METHODID_META:
          serviceImpl.meta((pbuf.App.BiobtreeMetaRequest) request,
              (io.grpc.stub.StreamObserver<pbuf.App.BiobtreeMetaResponse>) responseObserver);
          break;
        default:
          throw new AssertionError();
      }
    }

    @java.lang.Override
    @java.lang.SuppressWarnings("unchecked")
    public io.grpc.stub.StreamObserver<Req> invoke(
        io.grpc.stub.StreamObserver<Resp> responseObserver) {
      switch (methodId) {
        default:
          throw new AssertionError();
      }
    }
  }

  private static abstract class BiobtreeServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoFileDescriptorSupplier, io.grpc.protobuf.ProtoServiceDescriptorSupplier {
    BiobtreeServiceBaseDescriptorSupplier() {}

    @java.lang.Override
    public com.google.protobuf.Descriptors.FileDescriptor getFileDescriptor() {
      return pbuf.App.getDescriptor();
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.ServiceDescriptor getServiceDescriptor() {
      return getFileDescriptor().findServiceByName("BiobtreeService");
    }
  }

  private static final class BiobtreeServiceFileDescriptorSupplier
      extends BiobtreeServiceBaseDescriptorSupplier {
    BiobtreeServiceFileDescriptorSupplier() {}
  }

  private static final class BiobtreeServiceMethodDescriptorSupplier
      extends BiobtreeServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoMethodDescriptorSupplier {
    private final String methodName;

    BiobtreeServiceMethodDescriptorSupplier(String methodName) {
      this.methodName = methodName;
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.MethodDescriptor getMethodDescriptor() {
      return getServiceDescriptor().findMethodByName(methodName);
    }
  }

  private static volatile io.grpc.ServiceDescriptor serviceDescriptor;

  public static io.grpc.ServiceDescriptor getServiceDescriptor() {
    io.grpc.ServiceDescriptor result = serviceDescriptor;
    if (result == null) {
      synchronized (BiobtreeServiceGrpc.class) {
        result = serviceDescriptor;
        if (result == null) {
          serviceDescriptor = result = io.grpc.ServiceDescriptor.newBuilder(SERVICE_NAME)
              .setSchemaDescriptor(new BiobtreeServiceFileDescriptorSupplier())
              .addMethod(getGetMethod())
              .addMethod(getGetPageMethod())
              .addMethod(getFilterMethod())
              .addMethod(getMetaMethod())
              .build();
        }
      }
    }
    return result;
  }
}
