# Overview

This repo contains a simple CLI that performs getAccountInfo and getMultipleAccounts requests to Solana RPC providers, in order to gather statistics about them. The CLI will perform concurrent calls for each of these two methods, the amount of concurrent calls for each RPC can be declared in field `sampleSize` on the configuration file. More about `sampleSize` values in configuration file section.

## Environment variables

Take a look at file `.env.example`, 

```bash
LOG_LEVEL=TRACE

# Telegram configuration is not required but could be useful in case you want to receive a message
# when an error in the response of the RPC happens. The errors received are in the format
# 'RPC NAME error message', for example 'FLUX RPC request timeout'.
TGBOT_API_KEY="<TG_BOT_API_KEY>"
# All the subscribers specified as a comma separated that will receive notifications about errors on Telegram
# In case of channels should be specified with '@' before the channel id, for example @fluxnotify
# The channel must add the bot into its subscribers otherwise doesn't work
# In case of users is a number
NOTIFIER_SUBS="@channelusername"
```

## Available parameters

The available parameters are:

1. `config` to specify the configuration file path.
2. `interval` to define intervals on which perform the checks.

## Configuration file for RPCs

This script requires a json file to specify the RPCs to be requested and the accounts.

**For example**

```json
{
    "rpcs": [
        {
            "id": "Flux RPC",
            "endpoint": "https://eu.rpc.fluxbeam.xyz?key=<api-key>",
            "rateLimit": 15,
            "sampleSize": 1
        },
        {
            "id": "Helius",
            "endpoint": "https://mainnet.helius-rpc.com/?api-key=<api-key>",
            "rateLimit": 5,
            "sampleSize": 1
        }
    ],
    "accounts": [
        "Czfq3xZZDmsdGdUyrNLtRhGc47cXcZtLG4crryfu44zE",
        "3ucNos4NbumPLZNWztqGHNFFgkHeRMBQAVemeeomsUxv",
        "5rCf1DM8LjKTw4YqhnoLcngyZYeNnQqztScTogYHAS6"
    ]
}
```

The `sampleSize` defines how many concurrent requests we want to perform for each of the methods. For example if `sampleSize = 100` it will perform 100 concurrent requests for `getAccountInfo` and `getMultipleAccounts` on each RPC. This concurrent requests, will respect the `rateLimit` specified to avoid getting 429 status code which will add unecessary noise.

**NOTE**: Exept for `sampleSize`, which default value is 1, all the other fields are required. Some RPCs like Helius claim a
`10 rps` on the free plan but actually is `5 rps`, keep that in mind because sometimes you will get 429 if you specified the official rate limit specified by the RPC.

## How to run

Like you would run a normal Golang program. In this case given the is a CLI here is an example specifying the parameters.

```bash
go run main.go -config rpc.json -interval 1m
```

or if you compile it previously with `go build`

```bash
./rpc-notifier -config rpc.json -interval 1m
```