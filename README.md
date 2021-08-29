# Google Trends Go Scraper

## Run instructions

1. Make sure to install these Go packages 
```
go get -u github.com/groovili/gogtrends"
go get -u github.com/pkg/errors"
go get -u google.golang.org/protobuf
```
2. Also
```
go mod init project_name
go mod tidy
```
3. Build (and install any missing dependencies)
4. Run executable with list of words as command line arguments


## Run Example:

- List of keywords to query: defi, nft, Doja cat

Command to run:    
```
./gtrends defi nft "Doja Cat"
```