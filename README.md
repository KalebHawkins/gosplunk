# GoSplunk

GoSplunk is a tool written in `Go` to perform automated infrastructure deployments. 

## Infrastructure Automation

This tool is used to deploy automated infrastructure deployments for on-prem Splunk instances.

- [GoSplunk](#gosplunk)
  - [Infrastructure Automation](#infrastructure-automation)
  - [Overview](#overview)
  - [Architecture](#architecture)
  - [Prerequisites](#prerequisites)
    - [Ansible](#ansible)
    - [Go (Only if building from source)](#go-only-if-building-from-source)
      - [Go Compiler Installation](#go-compiler-installation)
      - [GoReleaser Installation](#goreleaser-installation)
  - [Building a Binary](#building-a-binary)
    - [Build from Source](#build-from-source)
    - [Install from Release](#install-from-release)
      - [RHEL/CentOS/Rocky Linux](#rhelcentosrocky-linux)
      - [Debian/Ubuntu](#debianubuntu)
      - [Build from Release](#build-from-release)
    - [The Configuration File](#the-configuration-file)
    - [Usage](#usage)


## Overview

The GoSplunk binary deploys virtual machines with the following spec depending on the package size you specify. The table below defines the size of each package that can be deployed.

| Package | CPU | MemoryGB | Application Disk (GB) |
|---------|-----|----------|-----------------------|
| Small   | 2   | 8        | 10                    |
| Medium  | 4   | 16       | 20                    |
| Large   | 8   | 32       | 40                    |

After the infrastructure has been deployed you can then use the GoSplunk tool to configure the virtual machine. See below sections for details on installation and usage.

## Architecture

```
                Virtual Machine
.----------------------------------------------------.
|  .-----------------.                               |
|  | [2 | 4 | 8] CPU |                               |
|  '-----------------'                               |
|  .-------------------------.                       |
|  | [8 | 16 | 32] GB Memory |                       |
|  '-------------------------'                       |
|                                                    |
|   _.-----._     _.-----._                          |
| .-         -. .-         -.                        |
| |-_       _-| |-_       _-|                        |
| |  ~-----~  | |  ~-----~  |                        |
| |System Disk| |  App Disk |                        |
| `._       _.' `._       _.'                        |
|    "-----"       "-----"                           |
|       ^             ^                              |
|       |             |                              |
|  .----|-------------'--------------------------.   |
|  |    '---------SCSI Controller 0              |   |
|  '---------------------------------------------'   |
'----------------------------------------------------'
```

## Prerequisites

### Ansible 

You will need to install ansible to run the configuration of the virtual machine. 

```bash
# Enable the Ansible Repo
subscription-manager repos --enable ansible-2.9-for-rhel-8-x86_64-rpms
# Install Ansible
dnf install ansible -y
```

Aside from ansible itself you will also need some ansible collections installed.

```bash
# For configuring LVM volumes
ansible-galaxy collection install community.general
# For firewalld
ansible-galaxy collection install ansible.posix
```

### Go (Only if building from source)

To build a binary from source you will need the `go compiler (v1.17+)` installed and `GoReleaser (v1.7.0+)`. 

> You will also need `git` installed. This documentation assumes that knowledge on how to download and use basic git commands. That process will not be covered here.

#### Go Compiler Installation

To install the `go compiler` use the following commands.

```bash
curl -LO https://go.dev/dl/go1.18.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.18.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

Verify the installation using the `go version` command.

#### GoReleaser Installation

To install `GoReleaser` run the following commands.

```bash
curl -L https://github.com/goreleaser/goreleaser/releases/download/v1.7.0/goreleaser_Linux_x86_64.tar.gz | tar zxv -C /usr/local/bin/
```

Verify the installation using the `goreleaser --version` command.


## Building a Binary

### Build from Source

To build a binary you need to pull the latest release. Move into the repository and run the build command. See below.

```bash
git clone https://github.com/KalebHawkins/gosplunk.git
cd gosplunk
goreleaser build --snapshot --rm-dist
```

Install the binary generated to your `PATH` using the following command. 

```bash
cp dist/v0.1.0_linux_amd64/gosplunk /usr/local/bin
```

Verify the installation.

```bash
# Note building this way will always reflect v0.0.0 but the commit should match the commit from the build output.
gosplunk version
```

This will start the build process creating a `dist` directory where your binary will be available. 

### Install from Release

#### RHEL/CentOS/Rocky Linux

To install from a release on RHEL based systems use the following commands.

```bash
$RELEASE=v1.0.0
curl -LO https://github.com/KalebHawkins/gosplunk/releases/download/$RELEASE/gosplunk_1.0.0_Linux_x86_64.rpm
dnf install -y gosplunk_1.0.0_Linux_x86_64.rpm
```

#### Debian/Ubuntu

To install from a release on Debian/Ubuntu based systems use the following commands.

```bash
$RELEASE=v1.0.0
curl -LO https://github.com/KalebHawkins/gosplunk/releases/download/$RELEASE/gosplunk_1.0.0_Linux_x86_64.deb
dpkg -i gosplunk_1.0.0_Linux_x86_64.deb
```

#### Build from Release

```bash
curl -LO https://github.com/KalebHawkins/gosplunk/releases/download/v1.0.0/gosplunk_1.0.0_Linux_x86_64.tar.gz
tar zxvf gosplunk_1.0.0_Linux_x86_64.tar.gz
cd gosplunk
cp gosplunk /usr/local/bin/
gosplunk version 
```

### The Configuration File

The configuration file can be downloaded along with a release version of the tar.gz source file or obtained seperately.

```bash
curl -LO https://raw.githubusercontent.com/KalebHawkins/gosplunk/v1.0.0/config.yml
```

You will need an ssh key to use for the new servers. If you don't already have one generated use the following command. 

> This key cannot have a passphrase or the configuration will fail. However, the key can be replaced after the build is complete.

```bash
ssh-keygen -t rsa -b 4096 -N '' -f /path/to/key
```

The `config.yml` file is documented to inform you what each field is for.

### Usage

Once you've modified the configuration it is time to build. There are 3 steps.

1. Build the infrastructure.
2. Copy ssh key to new servers.
3. Configure the new servers.

Use the configuration file obtained in the last step to generate your infrastructure.

```bash
# Generate a server or servers using a medium sized server build.
gosplunk deploy --config config.yml --medium
```

Once the servers are built you will need to copy your ssh key to the new servers.

```bash
ssh-copy-id -i /path/to/key root@<nodeName>
```

Configure the new servers.

```bash
gosplunk configure --config config.yml
```