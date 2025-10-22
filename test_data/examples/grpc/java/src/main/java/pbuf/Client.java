package pbuf;

import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;
import pbuf.App.BiobtreeGetRequest;
import pbuf.App.BiobtreeGetResponse;
import pbuf.BiobtreeServiceGrpc.BiobtreeServiceBlockingStub;

public class Client {

   public static void main(String args[]) {
     
      
      ManagedChannelBuilder chan = ManagedChannelBuilder.forAddress("localhost", 7777).usePlaintext();
      ManagedChannel ch= chan.build();
      BiobtreeServiceBlockingStub b= BiobtreeServiceGrpc.newBlockingStub(ch);
      
      // now make the request 
   
      BiobtreeGetRequest request=BiobtreeGetRequest.newBuilder().addKeywords("tpi1").build();
    
      BiobtreeGetResponse res= b.get(request);
      
      System.out.println("Number of results for tpi1 ->"+res.getResultsList().size());
      
   }
}
