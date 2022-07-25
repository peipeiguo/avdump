# avdump
Dump audio and video information of specified stream.

## Usage
```
git clone https://github.com/peipeiguo/avdump.git
cd avdump
go build
./avdump
Usage: avdump [-s stream_index] -i input_url
Options:
  -i string
    	Input URL, such as file path or URL
  -s int
    	Stream index to dump in input file, default is dump all streams (default -1)
./avdump -i rtmp://host:1935/app/stream
```
