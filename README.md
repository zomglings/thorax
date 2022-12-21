# thorax

thorax is a tool which synchronizes your Bugout journals into Segment. You can use Segment to load data from Bugout into any supported destination (e.g. Mixpanel).

## Installation

1. [Download the latest release](https://github.com/zomglings/thorax/releases/latest) - choose the
zip file appropriate to your operating system and CPU architecture.

2. Unzip the file. It will contain a directory called `thorax-<os>-<architecture>`. In that directory,
you will find a file called `thx`. This is the thorax command-line tool. You can either move it to
some directory on your global path (e.g. `/usr/local/bin`), or you can invoke it from the unzipped
directory. Your call.


If you would prefer to install thorax from source, you will need a working go toolchain. ([Click here to install Go](https://golang.org/dl/))

If you have Go set up:
```bash
go get github.com/zomglings/thorax
```

## Running thx

Run `thx` as follows:

```bash
thx \
    -N 100 \
    -journal "$BUGOUT_JOURNAL_ID" \
    -token "$BUGOUT_ACCESS_TOKEN" \
    -segment "$SEGMENT_WRITE_KEY" \
    -s 1
```

Parameters:

- `$BUGOUT_ACCESS_TOKEN`: You can get an access token for your Bugout account at https://bugout.dev/account/tokens.

- `$BUGOUT_JOURNAL_ID`: This is the Bugout journal from which you want to synchronize events into Segment.

- `$SEGMENT_WRITE_KEY`: Create a new source at https://app.segment.com. This is the write key associated with that source.

## Cursors

`thx` will print out cursors to the last processed event. If you want to process all events since your
last synchronization or if for some reason your job gets interrupted, you can pass the most recent
cursor when you restart the job using the `-cursor` argument.

For example:
```bash
thx \
    -N 100 \
    -journal "$BUGOUT_JOURNAL_ID" \
    -token "$BUGOUT_ACCESS_TOKEN" \
    -segment "$SEGMENT_WRITE_KEY" \
    -s 1 \
    -cursor "2021-03-15:29:28.171994+00:00"
```

You can synchronize cursors to your Bugout journal. Just add a `-cursorname <name>` flag to the `thx`
invocation. If you do this, you do not need to set `-cursor`. The most recent cursor with the given `<name>`
will automatically be used as a checkpoint.

## Getting help

- [Create an issue](https://github.com/zomglings/thorax/issues/new)

- [Ask us on Bugout Slack](https://join.slack.com/t/bugout-dev/shared_invite/zt-fhepyt87-5XcJLy0iu702SO_hMFKNhQ)


Please consider enabling crash reports. To do so, set the following environment variables when
you run `thx`:

```bash
export THORAX_REPORTING_ENABLED=yes
export THORAX_EMAIL=<your email>
```
