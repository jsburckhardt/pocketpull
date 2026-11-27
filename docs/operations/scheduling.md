# Cron scheduling

Install the binary at a stable absolute path yourself. The secret-file option
is recommended for cron, whose environment/working directory may differ from
your shell. From this checkout, prepare the configuration:

```sh
install -d -m 700 "$HOME/.config/pocketpull"
cp examples/pocketpull.yaml "$HOME/.config/pocketpull/config.yaml"
# Provision $HOME/.config/pocketpull/api-key through your secret workflow first.
chmod 600 "$HOME/.config/pocketpull/api-key"
unset POCKETPULL_API_KEY
/usr/local/bin/pocketpull --config "$HOME/.config/pocketpull/config.yaml"
```

Example crontab for a user whose home is `/home/collector` (hourly; native
nonwaiting overlap protection is built in):

```cron
15 * * * * /usr/local/bin/pocketpull --config /home/collector/.config/pocketpull/config.yaml 2>>/home/collector/pocketpull.log
```

Create the parent directories and provision the secret before enabling this
example. Cron and
external tooling own scheduling and log rotation. There is no application log
manager or telemetry exporter. At the default refresh interval this hourly
schedule can take up to 25 hours plus run time to capture an edit. To force an
immediate full refresh, invoke the same configuration with `--refresh-after 0`.
These examples do not imply a live Pocket pilot, release publication or cloud
validation.
