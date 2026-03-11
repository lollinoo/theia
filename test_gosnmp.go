package main

import (
"fmt"
"log"
"os"
"time"

"github.com/gosnmp/gosnmp"
)

func main() {
gs := &gosnmp.GoSNMP{
   "172.28.10.10",
     161,
ity: "public",
:   gosnmp.Version2c,
  time.Duration(5) * time.Second,
   gosnmp.NewLogger(log.New(os.Stdout, "", 0)),
}

err := gs.Connect()
if err != nil {
nect() err: %v", err)
}
defer gs.Conn.Close()

oids := []string{".1.3.6.1.2.1.1.1.0"}
result, err2 := gs.Get(oids)
if err2 != nil {
err: %v", err2)
}

for i, variable := range result.Variables {
tf("%d: oid: %s\n", i, variable.Name)
}
}
