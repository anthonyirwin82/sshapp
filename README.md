# sshapp

sshapp allows you to run an application when a user uses ssh to connect to your server.

The default port is `2222` you can change the port by using `sshapp --port 2245 --cmd "app-to-execute"`

## Running with SystemD

If you want to run sshapp with `systemd`, you can create a unit like so:

`/etc/systemd/system/myapp.service`:
```service
[Unit]
Description=My App
After=network.target

[Service]
Type=simple
User=myapp
Group=myapp
WorkingDirectory=/home/myapp/bin/
ExecStart=/home/myapp/bin/sshapp --port 2222 --cmd "app-to-execute"
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

To make the service run via SystemD

```bash
# need to run this every time you change the unit file
sudo systemctl daemon-reload

# start/restart/stop/etc:
sudo systemctl start myapp

# to load the sshapp server when the server boots/reboots
sudo systemctl enable myapp
```
