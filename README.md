- guarantees atleast once message delivery

InitConnection(addr string,port string,ourId string) , returns Connection object and error
(we call the above stuff to connect to someone!)

AcceptConnRequestLoop(ourId string) , go routine which keeps listening to port 8080 and creates Connection objects

`Connection object` will be referred as `conn` from now on here

conn.WriteToConn(data any) , returns error
conn.ReadFromConn() , returns any

conn can be fetched from the ConnId by asking the Manager by GetConnFromConnId(ConnId)


add to tests: send multiple handshakes, keep reading for a while, we should only get one ACK_HANDSHAKE from the peer



in connection failure scenarios:

readFromConnLoop would fail immediately as its actively listening to conn
writetoConnLoop might be blocked waiting on IO
writeACKtoConnLoop might be blocked waiting on IO

so the thing is, if something is waiting on IO, we can put a select and switch statements there with some closing channel



so essentially the read loop should fail first, 

then we call the retry from there, if we detect the connection actually broken,

be stop the writing routines whether they are blocked on some IO or not, and if they are inside the inner loop, they'll fail with an error by themselves, because the connection itself is broken

and the ones waiting on IO will be exited with the connectionDead channel closing