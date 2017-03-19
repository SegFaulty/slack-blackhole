# slack-blackhole

Remove messages and files in a certain duration for your Slack team.


## changes

- remove async message deleting, it was defect for already expired msgs
- the new usage is to start it regularly by cronjob and it will exits if all work is done
- change logtim to localtime not utc
- add the ability the delete msg in groups (private channels)
- only the groups of the token-holder are seen
- the tokenholder needs admin permission to delete msgs from other team-members
- change to slack-api-token-file for stronger security of the api-token (bash history, cron mails etc.)
- removed ENV config set
- add handled and deleted messages counter

## deployment

```
apt-get update
apt-get -y install golang
echo "export GOPATH=/usr/share/go/" >> /root/.profile
cd ~
git clone https://github.com/SegFaulty/slack-blackhole.git
cd slack-blackhole
# because of go build -i does not install the dependencies 
go get github.com/nlopes/slack
go build
# then create config and start
```

## todo

* todo only delete if "subtype":"bot_message" and config onlyBotMessages = true 


## Usage

a ttl of 0 means nothing will deleted

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
$ ./slack-blackhole --slack-api-token-file my-token-like-xoxp-aaa...txt --defaut-file-ttl $((86400*30)) --config-file config.json
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

