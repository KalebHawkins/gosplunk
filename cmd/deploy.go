/*
Copyright © 2022 Kaleb Hawkins <KalebHawkins@outlook.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tidwall/gjson"
	"github.com/vmware/govmomi/govc/cli"
	_ "github.com/vmware/govmomi/govc/device"
	_ "github.com/vmware/govmomi/govc/vm"
	_ "github.com/vmware/govmomi/govc/vm/disk"
)

type Package struct {
	Cpu       int
	MemoryMB  int
	AppDiskGB int
}

type VCenter struct {
	Url          string `yaml:"url"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	Template     string `yaml:"template"`
	Datastore    string `yaml:"datastore"`
	Network      string `yaml:"network"`
	ResourcePool string `yaml:"resourcepool"`
	Datacenter   string `yaml:"datacenter"`
}

type AHVCluster struct {
	URL                  string `yaml:"url"`
	Username             string `yaml:"username"`
	Password             string `yaml:"password"`
	Template             string `yaml:"template"`
	NetworkUUID          string `yaml:"networkUUID"`
	StorageContainerUUID string `yaml:"storageContainerUUID"`
	VolumeGroup          string `yaml:"volumeGroup"`
	Insecure             bool   `yaml:"insecure"`
	URI                  string
}

type Server struct {
	Name      string `yaml:"name"`
	IPAddress string `yaml:"ipaddress"`
	Netmask   string `yaml:"netmask"`
	Gateway   string `yaml:"gateway"`
}

const (
	AHV     = "AHV"
	VSPHERE = "VSPHERE"
)

var (
	smallPackage = Package{
		Cpu:       2,
		MemoryMB:  8096,
		AppDiskGB: 10,
	}

	mediumPackage = Package{
		Cpu:       4,
		MemoryMB:  16384,
		AppDiskGB: 20,
	}

	largePackage = Package{
		Cpu:       8,
		MemoryMB:  32768,
		AppDiskGB: 40,
	}
)

var (
	smallFlag  *bool
	mediumFlag *bool
	largeFlag  *bool
)

func init() {
	rootCmd.AddCommand(deployCmd)

	smallFlag = deployCmd.Flags().Bool("small", false, fmt.Sprintf("Deploy a server with %d CPUs, %.0fGB Memory, %dGB application disk",
		smallPackage.Cpu, float64(smallPackage.MemoryMB)/1024, smallPackage.AppDiskGB))

	mediumFlag = deployCmd.Flags().Bool("medium", false, fmt.Sprintf("Deploy a server with %d CPUs, %.0fGB Memory, %dGB application disk",
		mediumPackage.Cpu, float64(mediumPackage.MemoryMB)/1024, mediumPackage.AppDiskGB))

	largeFlag = deployCmd.Flags().Bool("large", false, fmt.Sprintf("Deploy a server with %d CPUs, %.0fGB Memory, %dGB application disk",
		largePackage.Cpu, float64(largePackage.MemoryMB)/1024, largePackage.AppDiskGB))
}

func checkFlags() error {
	if !*smallFlag && !*mediumFlag && !*largeFlag {
		return fmt.Errorf("--small, --medium, or --large flags must be set. Only one flag can be specified at a time")
	}
	if (*smallFlag && *mediumFlag) || (*smallFlag && *largeFlag) || (*mediumFlag && *largeFlag) {
		return fmt.Errorf("only one package size can sprecified")
	}

	return nil
}

// vSphere Functions
func setupEnviornment() error {
	var vc VCenter
	if err := viper.UnmarshalKey("vcenter", &vc); err != nil {
		return err
	}

	envMap := map[string]string{
		"GOVC_URL":           vc.Url,
		"GOVC_USERNAME":      vc.Username,
		"GOVC_PASSWORD":      vc.Password,
		"GOVC_TEMPLATE":      vc.Template,
		"GOVC_DATASTORE":     vc.Datastore,
		"GOVC_NETWORK":       vc.Network,
		"GOVC_RESOURCE_POOL": vc.ResourcePool,
		"GOVC_INSECURE":      "true",
		"GOVC_DATACENTER":    vc.Datacenter,
	}

	for k, v := range envMap {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}

	return nil
}

func runGOVC(args ...string) int {
	return cli.Run(args)
}

func cloneVM(p *Package, host *Server) error {
	vmCfg := []string{
		"vm.clone",
		"-vm", os.Getenv("GOVC_TEMPLATE"),
		"-on=false",
		fmt.Sprintf("-c=%d", p.Cpu),
		fmt.Sprintf("-m=%d", p.MemoryMB),
		fmt.Sprintf("-net=%s", os.Getenv("GOVC_NETWORK")),
		"-net.adapter=vmxnet3",
		host.Name,
	}

	if rtn := runGOVC(vmCfg...); rtn != 0 {
		return fmt.Errorf("failed to create virtual machine %s", host.Name)
	}

	return nil
}

func createVMDisk(p *Package, host *Server) error {
	diskCfg := []string{
		"vm.disk.create",
		"-vm", host.Name,
		"-name", fmt.Sprintf("%s/%s_001", host.Name, host.Name),
		"-size", fmt.Sprintf("%dG", p.AppDiskGB),
		"-thick=true",
	}

	if rtn := runGOVC(diskCfg...); rtn != 0 {
		return fmt.Errorf("failed to create disk for virtual machine %s", host.Name)
	}

	return nil
}

func setNicStartConnected(p *Package, host *Server) error {
	connectCfg := []string{
		"device.connect",
		"-vm", host.Name,
		"ethernet-0",
	}
	if rtn := runGOVC(connectCfg...); rtn != 0 {
		return fmt.Errorf("failed to set vmxnet3 adapter to start connected on virtual machine %s", host.Name)
	}

	return nil
}

func setIPAddress(host *Server) error {
	netCfg := []string{
		"vm.customize",
		"-vm", host.Name,
		"-ip", host.IPAddress,
		"-netmask", host.Netmask,
		"-gateway", host.Gateway,
	}

	if rtn := runGOVC(netCfg...); rtn != 0 {
		return fmt.Errorf("failed to set ip address of %s", host.Name)
	}

	return nil
}

func powerOn(host *Server) error {
	pwrCmd := []string{
		"vm.power", "-on", host.Name,
	}

	if rtn := runGOVC(pwrCmd...); rtn != 0 {
		return fmt.Errorf("failed to power on %s", host.Name)
	}

	return nil
}

func deployPackage(p *Package, host *Server) error {
	if err := cloneVM(p, host); err != nil {
		return err
	}
	if err := createVMDisk(p, host); err != nil {
		return err
	}
	if err := setNicStartConnected(p, host); err != nil {
		return err
	}
	if err := setIPAddress(host); err != nil {
		return err
	}
	if err := powerOn(host); err != nil {
		return err
	}

	return nil
}

// AHV Functions

func (ahv *AHVCluster) SanitizeURL() {
	var url string

	ahv.URI = "PrismGateway/services/rest/v2.0/"

	if ahv.URL[len(ahv.URL)-1:] == "/" {
		url = ahv.URL + ahv.URI
	} else {
		url = ahv.URL + "/" + ahv.URI
	}

	ahv.URL = url
}

func (ahv *AHVCluster) Get(Obj string) (string, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	URL := ahv.URL + Obj

	fmt.Printf("Getting data from URL: %s\n", URL)

	req, err := http.NewRequest("GET", URL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get data from %s: %s", URL, err)
	}

	req.Header.Set("ContentType", "application/json")
	req.SetBasicAuth(ahv.Username, ahv.Password)
	resp, err := client.Do(req)

	var bodyTextStr string
	if err != nil {
		return "", fmt.Errorf("failed to get response from %s: %s", URL, err)
	} else {
		bodyText, _ := ioutil.ReadAll(resp.Body)
		bodyTextStr = string(bodyText)
	}

	return bodyTextStr, err
}

func (ahv *AHVCluster) Post(Obj string, jsonByteData []byte) (string, int, error) {
	var bodyTextStr string
	var httpCode int

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	URL := ahv.URL + Obj

	fmt.Printf("Posting data to URL: %s\n", URL)

	payloadData := bytes.NewBuffer(jsonByteData)
	req, err := http.NewRequest("POST", URL, payloadData)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create new http request with payload: %s", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(ahv.Username, ahv.Password)

	resp, err := client.Do(req)

	if err != nil {
		return "", 0, fmt.Errorf("failed to get data from %s: %s", URL, err)
	} else {
		bodyText, _ := ioutil.ReadAll(resp.Body)
		httpCode = int(resp.StatusCode)
		bodyTextStr = string(bodyText)
	}

	return bodyTextStr, httpCode, err
}

// GenerateVMClonePayload payload JSON data for cloning the VM; takes clone Name as input
// Requires Network UUID of the network the Clone will be in
// Network UUID can be obtained usinag acli net.list on the CVM
func (ahv *AHVCluster) GenerateVMClonePayload(p *Package, srv *Server, vmUUID string) []byte {
	var postData string = fmt.Sprintf(`{
		"spec_list": [
			{
				"name": "%s",
				"memory_mb": %s,
				"num_vcpus": %s,
				"num_cores_per_vcpu": 1,
				"vm_nics": [
					{
						"adapter_type": "Vmxnet3",
						"network_uuid": "%s",
						"ip_address": "%s"
					}
				],
				"request_ip": false
			}
		]
	}`, srv.Name, fmt.Sprint(p.MemoryMB), fmt.Sprint(p.Cpu), ahv.NetworkUUID, srv.IPAddress)

	return []byte(postData)
}

// GetVMUUID gets uuid of the VM which is to be cloned
func (ahv *AHVCluster) GetVMUUID(vmName string) (string, error) {
	vmNameUUidMap := make(map[string]string)
	vmData, err := ahv.Get("vms")

	if err != nil {
		return "", fmt.Errorf("failed to get UUID of vm %s: %s", vmName, err)
	}

	vmNameJ := gjson.Get(vmData, "entities.#.name")
	vmUuidJ := gjson.Get(vmData, "entities.#.uuid")

	for i, name := range vmNameJ.Array() {
		for j, uuid := range vmUuidJ.Array() {
			if i == j {
				vmNameUUidMap[name.String()] = uuid.String()
			}
		}
	}
	return vmNameUUidMap[vmName], err
}

// CloneVM clones the source VM using POST v2 call to the /clone endpoint
// requires vm uuid , clone api endpoint and clone Name
func (ahv *AHVCluster) CloneVM(p *Package, srv *Server, vmUuid string) (string, int) {
	cloneByteData := ahv.GenerateVMClonePayload(p, srv, vmUuid)
	peObj := "vms/" + vmUuid + "/clone"
	resp, code, _ := ahv.Post(peObj, cloneByteData)
	return resp, code
}

func (ahv *AHVCluster) GenerateDiskPayload(p *Package) []byte {
	payload := fmt.Sprintf(`{
		"vm_disks": [
		  {
			"disk_address": {
			  "device_bus": "SCSI",
			  "device_index": 1,
			  "is_cdrom": false
			},
			"vm_disk_create": {
			  "size": %d,
			  "storage_container_uuid": "%s"
			}
		  }
		]
	  }`, p.AppDiskGB*1e+9, ahv.StorageContainerUUID)

	return []byte(payload)
}

func (ahv *AHVCluster) AttachDisk(p *Package, vmUUID string) (string, int) {
	diskPayload := ahv.GenerateDiskPayload(p)
	peObj := "vms/" + vmUUID + "/disks/attach"
	resp, code, _ := ahv.Post(peObj, diskPayload)
	return resp, code
}

func (ahv *AHVCluster) Deploy(p *Package, srv *Server) error {
	err := viper.UnmarshalKey("ahv", ahv)
	if err != nil {
		return err
	}

	ahv.SanitizeURL()

	templateUuid, err := ahv.GetVMUUID(ahv.Template)
	if err != nil {
		return err
	}

	fmt.Printf("Cloning %s from template %s\n", srv.Name, ahv.Template)
	resp, code := ahv.CloneVM(p, srv, templateUuid)

	if code >= 200 && code <= 299 {
		fmt.Printf("HTTP Code %d: Clone %s created from %s\n", code, srv.Name, ahv.Template)
	} else {
		fmt.Fprintf(os.Stderr, "failed to clone virtual machine %s from template %s\n", srv.Name, ahv.Template)
		fmt.Fprintf(os.Stderr, "HTTP Code: %d refer to https://portal.nutanix.com/page/documents/details?targetId=Objects-v2_0:v20-error-responses-c.html for more information.\n", code)
		fmt.Fprintf(os.Stderr, "Response: %s\n", resp)
		os.Exit(1)
	}

	fmt.Println("Waiting 30 seconds for VM creation to complete...")
	fmt.Println("30 seconds is just enough time to not really get started on anything but just enough time to finish reading this really pointless statement..")
	time.Sleep(30 * time.Second)

	fmt.Printf("Attaching app disk to %s\n", srv.Name)
	srvUUID, err := ahv.GetVMUUID(srv.Name)
	if err != nil {
		return err
	}

	resp, code = ahv.AttachDisk(p, srvUUID)
	if code >= 200 && code <= 299 {
		fmt.Printf("HTTP Code %d: application disk attach task created\n", code)
	} else {
		fmt.Fprintf(os.Stderr, "failed to attach application disk to %s\n", srv.Name)
		fmt.Fprintf(os.Stderr, "HTTP Code: %d refer to https://portal.nutanix.com/page/documents/details?targetId=Objects-v2_0:v20-error-responses-c.html for more information.\n", code)
		fmt.Fprintf(os.Stderr, "Response: %s\n", resp)
		os.Exit(1)
	}

	return nil
}

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy splunk infrastructure",
	Long:  `Deploy splunk infrastructure`,
	Run: func(cmd *cobra.Command, args []string) {

		if err := checkFlags(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		platform := checkPlatform()
		if platform == VSPHERE {
			fmt.Println("Setting up environment variables...")
			if err := setupEnviornment(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to setup enviornment variables: %s\n", err)
				os.Exit(1)
			}

			deployInfra(deployPackage)
			return
		}

		if platform == AHV {
			ahv := AHVCluster{}
			deployInfra(ahv.Deploy)
			return
		}

		if platform != AHV && platform != VSPHERE {
			fmt.Fprintf(os.Stderr, "invalid platform in configuration file\n")
			os.Exit(1)
		}
	},
}

func checkPlatform() string {
	if cfg := viper.Sub("ahv"); cfg != nil {
		return AHV
	}

	if cfg := viper.Sub("vcenter"); cfg != nil {
		return VSPHERE
	}

	return ""
}

func deployInfra(deploy func(*Package, *Server) error) {
	fmt.Println("Unmarshaling server structures...")

	var srvs []*Server
	if err := viper.UnmarshalKey("servers", &srvs); err != nil {
		fmt.Fprintf(os.Stderr, "failed to unmarshal servers: %s\n", err)
		os.Exit(1)
	}

	for _, srv := range srvs {

		fmt.Println("Using small package:", *smallFlag)
		fmt.Println("Using medium package:", *mediumFlag)
		fmt.Println("Using large package:", *largeFlag)

		if *smallFlag {
			if err := deploy(&smallPackage, srv); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		if *mediumFlag {
			if err := deploy(&mediumPackage, srv); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
		if *largeFlag {
			if err := deploy(&largePackage, srv); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}
}
