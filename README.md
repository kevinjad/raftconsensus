
# raftconsensus [under dev]

Go implementation of the famous distributed consensus algorithm called RAFT. I am following the raft entended paper for the implementation.




## Run Locally

Clone the project

```bash
  git clone https://github.com/kevinjad/raftconsensus.git
```

Go to the project directory

```bash
  cd raftconsensus
```

Build

```bash
  go build
```

run example

```bash
  ./raftconsensus <server_id>
```


## Usage/Examples

```go

package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/kevinjad/raftconsensus/raftconsensus"
)

func main() {
	source, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal("Provide source properly")
	}
	mapOfAddress := map[int]string{
		1: "127.0.0.1:8080",
		2: "127.0.0.1:8081",
		3: "127.0.0.1:8082",
		4: "127.0.0.1:8083",
	}

	s := raftconsensus.GetNewServer(source, mapOfAddress)
	s.Serve()
	time.Sleep(50 * time.Second)
}


```
