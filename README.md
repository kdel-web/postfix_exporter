# Prometheus Postfix exporter

```
PLEASE NOTE: The kumina/postfix_exporter has been updated and adapted to more accurately represent WebstaurantStore's implementation of Postfix and SMTP mailflow. 

The most significant changes are modifications to the original program's parsing of log lines, and the clean up of seemingly unmaintained third party dependencies, however, the changes are not relegated to these two examples.

Other source files and portions of code deemed irrelevant have been removed.
Documentation notes below may not be entirely correct. Please review source code for the most up to date information.
```


Prometheus metrics exporter for [the Postfix mail server](http://www.postfix.org/).
This exporter provides histogram metrics for the size and age of messages stored in
the mail queue. It extracts these metrics from Postfix by connecting to
a UNIX socket under `/var/spool`. It also counts events by parsing Postfix's
log entries, using regular expression matching. The log entries are retrieved from
the systemd journal, the Docker logs, or from a log file.
### The __log file__ (`/var/log/maillog`) continues to be the primary source of events/metrics.

## Options

These options can be used when starting the `postfix_exporter`

| Flag                     | Description                                          | Default                           |
|--------------------------|------------------------------------------------------|-----------------------------------|
| `--web.listen-address`   | Address to listen on for web interface and telemetry | `9154`** changing                 |
| `--web.telemetry-path`   | Path under which to expose metrics                   | `/metrics`                        |
| `--postfix.showq_path`   | Path at which Postfix places its showq socket        | `/var/spool/postfix/public/showq` |
| `--postfix.logfile_path` | Path where Postfix writes log entries                | `/var/log/mail.log`** changing    |
| `--log.unsupported`      | Log all unsupported lines                            | `false`** changing                |
| ~~`--docker.enable`      | Read from the Docker logs instead of a file          | `false`~~ removed                 |
| ~ docker.container.id  | The container to read Docker logs from               | `postfix`~ removed/irrelevant    |
| `--systemd.enable`       | Read from the systemd journal instead of file        | `false`** removing or updating    |
| `--systemd.unit`         | Name of the Postfix systemd unit                     | `postfix.service`**               |
| `--systemd.slice`        | Name of the Postfix systemd slice.                   | `""`**                            |
| `--systemd.journal_path` | Path to the systemd journal                          | `""`**                            |

~~## Events from Docker~~

~~Postfix servers running in a [Docker](https://www.docker.com/)~~
~~container can be monitored using the `--docker.enable` flag. The~~
~~default container ID is `postfix`, but can be customized with the~~
~~`--docker.container.id` flag.~~

~~The default is to connect to the local Docker, but this can be~~
~~customized using [the `DOCKER_HOST` and~~
~~similar](https://pkg.go.dev/github.com/docker/docker/client?tab=doc#NewEnvClient)~~
~~environment variables.~~

## Events from log file

The log file is tailed when processed. Rotating the log files while the exporter
is running is OK. The path to the log file is specified with the
`--postfix.logfile_path` flag.

## Events from systemd

Retrieval from the systemd journal is enabled with the `--systemd.enable` flag.
This overrides the log file setting.
It is possible to specify the unit (with `--systemd.unit`) or slice (with `--systemd.slice`).
Additionally, it is possible to read the journal from a directory with the `--systemd.journal_path` flag.

## Build options

Default the exporter is build with systemd journal functionality (but it is disabled at default).
Because the systemd headers are required for building with systemd, there is
an option to build the exporter without systemd. Use the build tag `nosystemd`.

```
go build -tags nosystemd
```
