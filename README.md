# slack-blackhole

Remove messages and files in a certain duration for your Slack team.


## changes

- remove async message deleting, it was defect for already expired msgs
- the new usage is to start it regularly by cronjob and it will exits if all work is done
- change logtim to localtime not utc
- add the ability the delete msg in groups (private channels)
- only the groups of the token-holder are seen
- the tokenholder needs admin permission to delete msgs from other team-members

## deployment

```
apt-get update
apt-get -y install golang
echo "export GOPATH=/usr/share/go/" >> /root/.profile
cd ~
git clone https://github.com/SegFaulty/slack-blackhole.git
go build 
```


## Usage

```
$ go build
$ cat config.json
[
        {
                "channel": "dev_null",
                "message_ttl": 600,
                "file_ttl": 600
        },
        {
                "channel": "dev_null_daily",
                "message_ttl": 86400,
                "file_ttl": 86400
        }
]
$ ./slack-blackhole --slack-api-token xoxp-aaa... --defaut-file-ttl $((86400*30)) --config-file config.json
```

### Other options

```
$ ./slack-blackhole --help
Usage of ./slack-blackhole:
  -config-file string
        Configuration file
  -debug
        Debug on
  -debug-slack
        Debug on for Slack
  -default-file-ttl int
        TTL of files for all channel
  -default-message-ttl int
        TTL of messages for all channel
  -dry-run
        Do not delete messages/files
  -slack-api-interval int
        Interval (sec) for api call (default 3)
  -slack-api-token string
        Slack API token
```

All options can be set as environment variables.  Each environment variable
has `BLACKHOLE_` prefix like `BLACKHOLE_DEBUG` for `--debug`.

## Author

Katsuyuki Tateishi <kt@wheel.jp>

