# Internet Connected Beverage Monitor server

This is the source for the ICBM server code, as used to support the [Lunarville beer fridge](http://lunarville.org).

## Development
- [ ] Install the [Go compiler](https://golang.org/dl/) for your operating system (Windows, macOS, or Linux).
- [ ] Install an IDE of your choice (eg, VSCode, Vim, etc.)
- [ ] Make changes, test them (`go test`, `go build`)
- [ ] When ready to deploy it to production, cross compile the code for the target system OS and architecture (usually linux/amd64): eg, `env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build` on macOS or Linux.

If you're compiling this code on Windows, there's [other necessary steps](https://stackoverflow.com/questions/20829155/how-to-cross-compile-from-windows-to-linux) to set these three environment variables correctly. (Basically, either set those variables as Administrator in the control panel, or write a .bat file to do the compilation for you.)

As part of the compilation process, all assets will be embedded into the final executable as runtime resources. This includes:
  - the contents of the `static` folder intended for serving files to web browsers,
  - the `template` folder which contains internal templates used to render diagnostic information,
  - the `icbm.service` file which is used to install the ICBM service on a new machine,
  - and `icbmuserdb.json` which contains valid API keys to authorize client systems to post data.

See `service/install.go`, build.sh, and deploy.sh for further details.

## Deployment

All assets and prerequisites are bundled into a single executable file. Nothing else needs to be copied around or created.

The compiled executable for the server expects to be run on a systemd-based Linux system, such as a recent version of Debian or Ubuntu Linux. An appropriate Virtual Machine (VM) can be set up on [Digital Ocean](https://www.digitalocean.com/) or [Scaleway](https://www.scaleway.com/en/) in about five minutes of clicking, for $5 a month. 10GB of storage is plenty.

Other steps:
- [ ] Register a DNS name for the icbm server, such as `icbm.lunarville.org`, and point it at that VM's public IP address.
- [ ] Ensure that the `static/index.html` file in the source code refers to the new DNS name, in particular in the `loadChartData` function.
- [ ] Ensure the ICBM client side data acquisition code has the new DNS name for posting data samples. In Lunarville's case this is the Raspberry Pi Jeff put together.
- [ ] Update the `icbm.service` file to contain the new SSL host name. In particular, the ExecStart line would read something like: `ExecStart=/svc/icbm/web -http :80 -https :443 -tlshosts icbm.lunarville.org,lunarville.org`
- [ ] Compile a new version of the server code per the section above.
- [ ] Copy the linux/amd64 compiled executable to the machine to any target directory (scp, or Putty's pscp.exe on Windows.)
- [ ] Invoke the program as root with `-install` to set up the Digital Ocean VM as an ICBM server.

Invoking the compiled code with `-install` on a target systemd based linux system will doing everything necessary to get the server code running. In particular, it will automatically:
  - create a dedicated user named `svc-icbm` to run the server,
  - create a home directory in `/svc/icbm` to contain all the data, 
  - create the necessary systemd service file to run the ICBM server at system boot,
  - enable the service, 
  - and start it.

Invoking the /svc/icbm/web command with `-uninstall` will remove the user and disable the service, but leave the folders and data in place.

Historical data is stored in /svc/icbm/data. Secrets necessary for the ICBM server to automatically renew its SSL certificates are stored in the `/svc/icbm/secrets` folder.

## Monitoring

To see if the server is healthy run the `test-icbm.sh` script on Linux, macOS, or WSL2. Adjust the target server names as necessary. If all is good it will print a series of lines, all starting with "PASS".

## Development

I tried to keep the source boring and easy to read. At some point replacing the client-side graphing JS with server-rendered graphs would be a fine thing. There's also a `cull` routine I may commit which removes any data points which don't change the graph.

## Redundancy

The client side retries, which in practice is good enough should something happen to the server's availability for a window. It would not be hard to run this on two or more systems and have them sync against each other, but that's multiple kinds of overkill for a beer fridge. Though I may spend an evening adding backup and restore to an S3 compatible storage service at some point, and point the front-end HTML at a prerendered graph there.

## License

tl;dr It's lax. This code is under the "do pretty much whatever you like, without guarantees" MIT license. In bullet point form:

**Permissions**
  - Commercial use: The licensed material and derivatives may be used for commercial purposes.
  - Distribution:   The licensed material may be distributed.
  - Modification:   The licensed material may be modified.
  - Private use:    The licensed material may be used and modified in private.

**Conditions**
  - A copy of the license and copyright notice must be included with the licensed material in source form, but is not required for binaries.

**Limitations**
  - Liability:     This license includes a limitation of liability.
  - Warranty:      This license explicitly states that it does NOT provide any warranty.

