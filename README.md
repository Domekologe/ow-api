<p align="center">
<img src="service/static/assets/logo.png" width="310" height="71" border="0" alt="ovrstat">
<br>
<a href="https://goreportcard.com/badge/github.com/Domekologe/ow-api"><img src="https://goreportcard.com/badge/github.com/Domekologe/ow-api" alt="Go Report Card"></a>
<a href="https://pkg.go.dev/badge/github.com/Domekologe/ow-api"><img src="https://pkg.go.dev/badge/github.com/Domekologe/ow-api" alt="GoDoc"></a>
</p>

## Latest Changes
ATTENTION!
With the latest update `namecardID` and `namecardTitle` are removed! (Thanks to Blizzard for again making changes)

## General
After ovrstat is obsolete/archived and OW-API didn't get specific values I made an functional version here for my own.

`ovrstat` is a simple web scraper for the Overwatch stats site that parses and serves the data retrieved as JSON. Included is the go package used to scrape the info for usage in any go binary. This is a single endpoint web-scraping API that takes the full payload of information that we retrieve from Blizzard and passes it through to you in a single response. Things like caching and splitting data across multiple responses could likely improve performance, but in pursuit of keeping things simple, ovrstat does not implement them.

## Configuration
The application can be configured via environment variables:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `PORT` | The HTTP port the server listens on | `8080` |

### Changing the Port
**Linux (Bash):**
```bash
export PORT=9000
./ow-api
```

**Windows (PowerShell):**
```powershell
$env:PORT="9000"
.\ow-api.exe
```

**Direct in main.go File:**
service.Start(getenv("PORT", "<PORT>")) 
Change the "<PORT> Value

## Installation & Usage

### 1. Build from Source
Ensure you have [Go](https://go.dev/dl/) installed (minimum 1.24).

```bash
# Clone the repository
git clone https://github.com/Domekologe/ow-api.git
cd ow-api

# Build the binary
go build .
```
This will create a binary named `ow-api` (or `ow-api.exe` on Windows).

### 2. Run the Application

**Linux / macOS:**
```bash
./ow-api
```

**Windows:**
```powershell
.\ow-api.exe
```
The server will start on port 8080 (default).

### 3. Docker
You can run the official image directly:

```bash
docker run -p 8080:8080 domekologe/owapi:latest
```

Or build it locally:
```bash
docker build -t owapi .
docker run -p 8080:8080 owapi
```

## API Usage

Below is an example of using the REST endpoint (note: CASE matters for the username/tag):
```
http://localhost:8080/stats/pc/Viz-1213
http://localhost:8080/stats/console/Viz-1213
```

### Using Go to retrieve Stats

```go
package main

import (
	"log"

	"github.com/Domekologe/ow-api/ovrstat"
)

func main() {
	log.Println(ovrstat.Stats(ovrstat.PlatformPC, "Viz-1213"))
    log.Println(ovrstat.Stats(ovrstat.PlatformConsole, "Viz-1213"))
}
```

## Disclaimer
ovrstat isn’t endorsed by Blizzard and doesn’t reflect the views or opinions of Blizzard or anyone officially involved in producing or managing Overwatch. Overwatch and Blizzard are trademarks or registered trademarks of Blizzard Entertainment, Inc. Overwatch © Blizzard Entertainment, Inc.

The BSD 3-clause License
========================

Copyright (c) 2023, s32x, Domekologe, ToasterUwU. All rights reserved.

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

 - Redistributions of source code must retain the above copyright notice,
   this list of conditions and the following disclaimer.

 - Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

 - Neither the name of ovrstat nor the names of its contributors may
   be used to endorse or promote products derived from this software without
   specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR
ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON
ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
