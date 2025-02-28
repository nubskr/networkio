- guarantees atleast once message delivery

InitConnection(addr string,port string,ourId string) , returns Connection object and error
(we call the above stuff to connect to someone!)

AcceptConnRequestLoop(ourId string) , go routine which keeps listening to port 8080 and creates Connection objects

`Connection object` will be referred as `conn` from now on here

conn.WriteToConn(data any) , returns error
conn.ReadFromConn() , returns any

conn can be fetched from the ConnId by asking the Manager by GetConnFromConnId(ConnId)