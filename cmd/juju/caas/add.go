// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	jujucmdcloud "github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

var logger = loggo.GetLogger("juju.cmd.juju.k8s")

type CloudMetadataStore interface {
	ParseCloudMetadataFile(path string) (map[string]jujucloud.Cloud, error)
	ParseOneCloud(data []byte) (jujucloud.Cloud, error)
	PublicCloudMetadata(searchPaths ...string) (result map[string]jujucloud.Cloud, fallbackUsed bool, _ error)
	PersonalCloudMetadata() (map[string]jujucloud.Cloud, error)
	WritePersonalCloudMetadata(cloudsMap map[string]jujucloud.Cloud) error
}

// AddCloudAPI - Implemented by cloudapi.Client.
type AddCloudAPI interface {
	AddCloud(jujucloud.Cloud) error
	AddCredential(tag string, credential jujucloud.Credential) error
	Close() error
}

// BrokerGetter returns caas broker instance.
type BrokerGetter func(cloud jujucloud.Cloud, credential jujucloud.Credential) (k8sBrokerRegionLister, error)

type k8sBrokerRegionLister interface {
	ListHostCloudRegions() (set.Strings, error)
}

var usageAddCAASSummary = `
Adds a k8s endpoint and credential to Juju.`[1:]

var usageAddCAASDetails = `
Creates a user-defined cloud and populate the selected controller with the k8s
cloud details. Speficify non default kubeconfig file location using $KUBECONFIG
environment variable or pipe in file content from stdin.

The config file can contain definitions for different k8s clusters,
use --cluster-name to pick which one to use.
It's also possible to select a context by name using --context-name.

When running add-k8s on JAAS and the cloud/region cannot be detected automatically,
use --region <cloudType/region> to specify the host cloud type and region.

When adding a GKE cluster, you can use the --gke option to interactively be stepped
through the registration process, or you can supply the necessary parameters directly.

Examples:
    juju add-k8s myk8scloud
    juju add-k8s --context-name mycontext myk8scloud
    juju add-k8s myk8scloud --region <cloudType/region>

    KUBECONFIG=path-to-kubuconfig-file juju add-k8s myk8scloud --cluster-name=my_cluster_name
    kubectl config view --raw | juju add-k8s myk8scloud --cluster-name=my_cluster_name

    juju add-k8s --gke myk8scloud
    juju add-k8s --gke --project=myproject myk8scloud
    juju add-k8s --gke --credential=myaccount --project=myproject myk8scloud
    juju add-k8s --gke --credential=myaccount --project=myproject --region=someregion myk8scloud

See also:
    remove-k8s
`

// AddCAASCommand is the command that allows you to add a caas and credential
type AddCAASCommand struct {
	modelcmd.ControllerCommandBase

	// caasName is the name of the caas to add.
	caasName string

	// caasType is the type of CAAS being added.
	caasType string

	// clusterName is the name of the cluster (k8s) or credential to import.
	clusterName string

	// contextName is the name of the contex to import.
	contextName string

	// project is the project id for the cluster.
	project string

	// credential is the credential to use when accessing the cluster.
	credential string

	// hostCloudRegion is the cloud region that the nodes of cluster (k8s) are running in.
	// The format is <cloudType/region>.
	hostCloudRegion string

	// brokerGetter returns caas broker instance.
	brokerGetter BrokerGetter

	gke        bool
	k8sCluster k8sCluster

	cloudMetadataStore    CloudMetadataStore
	fileCredentialStore   jujuclient.CredentialStore
	addCloudAPIFunc       func() (AddCloudAPI, error)
	newClientConfigReader func(string) (clientconfig.ClientConfigFunc, error)

	getAllCloudDetails func() (map[string]*jujucmdcloud.CloudDetails, error)
}

// NewAddCAASCommand returns a command to add caas information.
func NewAddCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	cmd := &AddCAASCommand{
		cloudMetadataStore:  cloudMetadataStore,
		fileCredentialStore: jujuclient.NewFileCredentialStore(),
		newClientConfigReader: func(caasType string) (clientconfig.ClientConfigFunc, error) {
			return clientconfig.NewClientConfigReader(caasType)
		},
	}
	cmd.addCloudAPIFunc = func() (AddCloudAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cloudapi.NewClient(root), nil
	}

	cmd.brokerGetter = newK8sBrokerGetter(cmd.NewAPIRoot)
	cmd.getAllCloudDetails = jujucmdcloud.GetAllCloudDetails
	return modelcmd.WrapController(cmd)
}

// Info returns help information about the command.
func (c *AddCAASCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add-k8s",
		Args:    "<k8s name>",
		Purpose: usageAddCAASSummary,
		Doc:     usageAddCAASDetails,
	})
}

// SetFlags initializes the flags supported by the command.
func (c *AddCAASCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.clusterName, "cluster-name", "", "Specify the k8s cluster to import")
	f.StringVar(&c.contextName, "context-name", "", "Specify the k8s context to import")
	f.StringVar(&c.hostCloudRegion, "region", "", "kubernetes cluster cloud and/or region")
	f.StringVar(&c.project, "project", "", "project to which the cluster belongs")
	f.StringVar(&c.credential, "credential", "", "the credential to use when accessing the cluster")
	f.BoolVar(&c.gke, "gke", false, "used when adding a GKE cluster")
}

// Init populates the command with the args from the command line.
func (c *AddCAASCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.Errorf("missing k8s name.")
	}
	c.caasType = "kubernetes"
	c.caasName = args[0]

	if c.contextName != "" && c.clusterName != "" {
		return errors.New("only specify one of cluster-name or context-name, not both")
	}
	if c.gke {
		if c.contextName != "" {
			return errors.New("do not specify context name when adding a GKE cluster")
		}
	} else {
		if c.project != "" {
			return errors.New("do not specify project unless adding a GKE cluster")
		}
		if c.credential != "" {
			return errors.New("do not specify credential unless adding a GKE cluster")
		}
	}

	return cmd.CheckEmpty(args[1:])
}

// getStdinPipe returns nil if the context's stdin is not a pipe.
func getStdinPipe(ctx *cmd.Context) (io.Reader, error) {
	if stdIn, ok := ctx.Stdin.(*os.File); ok && !terminal.IsTerminal(int(stdIn.Fd())) {
		// stdIn from pipe but not terminal
		stat, err := stdIn.Stat()
		if err != nil {
			return nil, err
		}
		content, err := ioutil.ReadAll(stdIn)
		if err != nil {
			return nil, err
		}
		if (stat.Mode()&os.ModeCharDevice) == 0 && len(content) > 0 {
			// workaround to get piped stdIn size because stat.Size() always == 0
			return bytes.NewReader(content), nil
		}
	}
	return nil, nil
}

func (c *AddCAASCommand) newCloudCredentialFromKubeConfig(reader io.Reader, contextName, clusterName string) (jujucloud.Cloud, jujucloud.Credential, clientconfig.Context, error) {
	var credential jujucloud.Credential
	var context clientconfig.Context
	newCloud := jujucloud.Cloud{
		Name:            c.caasName,
		Type:            c.caasType,
		HostCloudRegion: c.hostCloudRegion,
	}
	clientConfigFunc, err := c.newClientConfigReader(c.caasType)
	if err != nil {
		return newCloud, credential, context, errors.Trace(err)
	}
	caasConfig, err := clientConfigFunc(reader, contextName, clusterName, clientconfig.EnsureK8sCredential)
	if err != nil {
		return newCloud, credential, context, errors.Trace(err)
	}
	logger.Debugf("caasConfig: %+v", caasConfig)

	if len(caasConfig.Contexts) == 0 {
		return newCloud, credential, context, errors.Errorf("No k8s cluster definitions found in config")
	}

	context = caasConfig.Contexts[reflect.ValueOf(caasConfig.Contexts).MapKeys()[0].Interface().(string)]

	credential = caasConfig.Credentials[context.CredentialName]
	newCloud.AuthTypes = []jujucloud.AuthType{credential.AuthType()}
	currentCloud := caasConfig.Clouds[context.CloudName]
	newCloud.Endpoint = currentCloud.Endpoint

	cloudCAData, ok := currentCloud.Attributes["CAData"].(string)
	if !ok {
		return newCloud, credential, context, errors.Errorf("CAData attribute should be a string")
	}
	newCloud.CACertificates = []string{cloudCAData}
	return newCloud, credential, context, nil
}

func (c *AddCAASCommand) getConfigReader(ctx *cmd.Context) (io.Reader, string, error) {
	if !c.gke {
		rdr, err := getStdinPipe(ctx)
		return rdr, c.clusterName, err
	}
	p := &clusterParams{
		name:       c.clusterName,
		region:     c.hostCloudRegion,
		project:    c.project,
		credential: c.credential,
	}
	// TODO - add support for AKS etc
	cluster := c.k8sCluster
	if cluster == nil {
		cluster = newGKECluster()
	}

	// If any items are missing, prompt for them.
	if p.name == "" || p.project == "" || p.region == "" {
		var err error
		p, err = cluster.interactiveParams(ctx, p)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
	}
	c.clusterName = p.name
	c.hostCloudRegion = cluster.cloud() + "/" + p.region
	return cluster.getKubeConfig(p)
}

// Run is defined on the Command interface.
func (c *AddCAASCommand) Run(ctx *cmd.Context) error {
	if err := c.verifyName(c.caasName); err != nil {
		return errors.Trace(err)
	}
	rdr, clusterName, err := c.getConfigReader(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if closer, ok := rdr.(io.Closer); ok {
		defer closer.Close()
	}
	newCloud, credential, context, err := c.newCloudCredentialFromKubeConfig(rdr, c.contextName, clusterName)
	if err != nil {
		return errors.Trace(err)
	}

	cloudClient, err := c.addCloudAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer cloudClient.Close()

	if err := c.addCloudToControllerWithRegion(cloudClient, newCloud); err != nil {
		if !params.IsCodeCloudRegionRequired(err) {
			return errors.Trace(err)
		}
		// try to fetch cloud and region then retry.
		cloudRegion, err := c.getClusterRegion(ctx, newCloud, credential)
		errMsg := `
	JAAS requires cloud and region information. But it's
	not possible to fetch cluster region in this case,
	please use --region to specify the cloud/region manually.
	`[1:]
		if err != nil {
			return errors.Annotate(err, errMsg)
		}
		if cloudRegion == "" {
			return errors.NewNotValid(nil, errMsg)
		}
		newCloud.HostCloudRegion = cloudRegion
		if err := c.addCloudToControllerWithRegion(cloudClient, newCloud); err != nil {
			return errors.Trace(err)
		}
	}

	if err := addCloudToLocal(c.cloudMetadataStore, newCloud); err != nil {
		return errors.Trace(err)
	}

	if err := c.addCredentialToLocal(c.caasName, credential, context.CredentialName); err != nil {
		return errors.Trace(err)
	}

	if err := c.addCredentialToController(cloudClient, credential, context.CredentialName); err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintf(ctx.Stdout, "k8s substrate %q added as cloud %q\n", clusterName, c.caasName)

	return nil
}

func (c *AddCAASCommand) addCloudToControllerWithRegion(apiClient AddCloudAPI, newCloud jujucloud.Cloud) (err error) {
	if newCloud.HostCloudRegion != "" {
		hostCloudRegion, err := c.validateCloudRegion(newCloud.HostCloudRegion)
		if err != nil {
			return errors.Trace(err)
		}
		newCloud.HostCloudRegion = hostCloudRegion
	}
	if err := addCloudToController(apiClient, newCloud); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func newK8sBrokerGetter(rootAPIGetter func() (api.Connection, error)) BrokerGetter {
	return func(cloud jujucloud.Cloud, credential jujucloud.Credential) (k8sBrokerRegionLister, error) {
		conn, err := rootAPIGetter()
		if err != nil {
			return nil, errors.Trace(err)
		}
		modelAPI := modelconfig.NewClient(conn)
		defer modelAPI.Close()

		// Use the controller model config for constructing the Juju k8s client.
		attrs, err := modelAPI.ModelGet()
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfg, err := config.New(config.NoDefaults, attrs)
		if err != nil {
			return nil, errors.Trace(err)
		}

		cloudSpec, err := environs.MakeCloudSpec(cloud, "", &credential)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return caas.New(environs.OpenParams{Cloud: cloudSpec, Config: cfg})
	}
}

func parseCloudRegion(cloudRegion string) (string, string, error) {
	fields := strings.SplitN(cloudRegion, "/", 2)
	if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
		return "", "", errors.NotValidf("cloud region %s", cloudRegion)
	}
	return fields[0], fields[1], nil
}

func (c *AddCAASCommand) validateCloudRegion(cloudRegion string) (_ string, err error) {
	defer errors.DeferredAnnotatef(&err, "validating cloud region %q", cloudRegion)

	cloudNameOrType, region, err := parseCloudRegion(cloudRegion)
	if err != nil {
		return "", errors.Annotate(err, "parsing cloud region")
	}

	clouds, err := c.getAllCloudDetails()
	if err != nil {
		return "", errors.Annotate(err, "listing cloud regions")
	}
	for name, details := range clouds {
		// User may have specified cloud name or type so match on both.
		if name == cloudNameOrType || details.CloudType == cloudNameOrType {
			for k := range details.RegionsMap {
				if k == region {
					logger.Debugf("cloud region %q is valid", cloudRegion)
					return details.CloudType + "/" + region, nil
				}
			}
		}
	}
	return "", errors.NotValidf("cloud region %s", cloudRegion)
}

func (c *AddCAASCommand) getClusterRegion(
	ctx *cmd.Context,
	cloud jujucloud.Cloud,
	credential jujucloud.Credential,
) (string, error) {
	broker, err := c.brokerGetter(cloud, credential)
	if err != nil {
		return "", errors.Trace(err)
	}

	interrupted := make(chan os.Signal, 1)
	defer close(interrupted)
	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)

	result := make(chan string, 1)
	errChan := make(chan error, 1)
	go func() {
		cloudRegions, err := broker.ListHostCloudRegions()
		if err != nil {
			errChan <- err
		}
		if cloudRegions == nil || cloudRegions.Size() == 0 {
			result <- ""
		} else {
			// we currently assume it's always a single region cluster.
			result <- cloudRegions.SortedValues()[0]
		}
	}()

	timeout := 30 * time.Second
	defer fmt.Fprintln(ctx.Stdout, "")
	for {
		select {
		case <-time.After(1 * time.Second):
			fmt.Fprintf(ctx.Stdout, ".")
		case <-interrupted:
			ctx.Infof("ctrl+c detected, aborting...")
			return "", nil
		case <-time.After(timeout):
			return "", errors.Timeoutf("timeout after %v", timeout)
		case err := <-errChan:
			return "", err
		case cloudRegion := <-result:
			return cloudRegion, nil
		}
	}
}

func (c *AddCAASCommand) verifyName(name string) error {
	public, _, err := c.cloudMetadataStore.PublicCloudMetadata()
	if err != nil {
		return err
	}
	msg, err := nameExists(name, public)
	if err != nil {
		return errors.Trace(err)
	}
	if msg != "" {
		return errors.Errorf(msg)
	}
	return nil
}

// nameExists returns either an empty string if the name does not exist, or a
// non-empty string with an error message if it does exist.
func nameExists(name string, public map[string]jujucloud.Cloud) (string, error) {
	if _, ok := public[name]; ok {
		return fmt.Sprintf("%q is the name of a public cloud", name), nil
	}
	builtin, err := common.BuiltInClouds()
	if err != nil {
		return "", errors.Trace(err)
	}
	if _, ok := builtin[name]; ok {
		return fmt.Sprintf("%q is the name of a built-in cloud", name), nil
	}
	return "", nil
}

func addCloudToLocal(cloudMetadataStore CloudMetadataStore, newCloud jujucloud.Cloud) error {
	personalClouds, err := cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if personalClouds == nil {
		personalClouds = make(map[string]jujucloud.Cloud)
	}
	personalClouds[newCloud.Name] = newCloud
	return cloudMetadataStore.WritePersonalCloudMetadata(personalClouds)
}

func addCloudToController(apiClient AddCloudAPI, newCloud jujucloud.Cloud) error {
	err := apiClient.AddCloud(newCloud)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToLocal(cloudName string, newCredential jujucloud.Credential, credentialName string) error {
	newCredentials := &jujucloud.CloudCredential{
		AuthCredentials: make(map[string]jujucloud.Credential),
	}
	newCredentials.AuthCredentials[credentialName] = newCredential
	err := c.fileCredentialStore.UpdateCredential(cloudName, *newCredentials)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToController(apiClient AddCloudAPI, newCredential jujucloud.Credential, credentialName string) error {
	currentAccountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return errors.Trace(err)
	}

	cloudCredTag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s",
		c.caasName, currentAccountDetails.User, credentialName))

	if err := apiClient.AddCredential(cloudCredTag.String(), newCredential); err != nil {
		return errors.Trace(err)
	}
	return nil
}
