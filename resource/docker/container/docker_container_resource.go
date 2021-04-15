package container

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alessio/shellescape"
	"github.com/docker/docker/api/types"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/numtide/terraform-provider-linuxbox/sshsession"
	"github.com/pkg/errors"
)

func Resource() *schema.Resource {
	return &schema.Resource{
		Create: resourceCreate,
		Read:   resourceRead,
		Update: resourceUpdate,
		Delete: resourceDelete,

		Schema: map[string]*schema.Schema{
			"ssh_key": &schema.Schema{
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
			},

			"ssh_user": &schema.Schema{
				Type:     schema.TypeString,
				Required: false,
				Default:  "root",
				Optional: true,
			},

			"host_address": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"image_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"ports": &schema.Schema{
				Type: schema.TypeSet,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
			},

			"privileged": &schema.Schema{
				Type:     schema.TypeBool,
				Default:  false,
				Optional: true,
			},

			"caps": &schema.Schema{
				Type: schema.TypeSet,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
			},

			"volumes": &schema.Schema{
				Type: schema.TypeSet,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
			},

			"labels": &schema.Schema{
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
			},

			"env": &schema.Schema{
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
			},

			"restart": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"network": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"args": &schema.Schema{
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
			},

			"name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},

			"container_id": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"log_driver": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "json-file",
			},

			"log_opts": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
				Default:  map[string]interface{}{},
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"memory": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
				Default:  0,
			},
		},
	}
}

func resourceCreate(d *schema.ResourceData, m interface{}) error {

	imageID := d.Get("image_id").(string)

	cmd := []string{
		"docker",
		"run",
		"-d",
	}

	restart, restartSet := d.GetOkExists("restart")

	if restartSet {
		cmd = append(cmd, "--restart", shellescape.Quote(restart.(string)))
	}

	name, nameSet := d.GetOkExists("name")

	if nameSet {
		cmd = append(cmd, "--name", shellescape.Quote(name.(string)))
	}

	privileged := d.Get("privileged").(bool)

	if privileged {
		cmd = append(cmd, "--privileged")
	}

	network, networkSet := d.GetOkExists("network")

	if networkSet {
		cmd = append(cmd, "--network", shellescape.Quote(network.(string)))
	}

	labelsMap := d.Get("labels").(map[string]interface{})

	for k, v := range labelsMap {
		cmd = append(cmd, "-l", fmt.Sprintf("%s=%s", shellescape.Quote(k), shellescape.Quote(v.(string))))
	}

	logDriver := d.Get("log_driver").(string)

	cmd = append(cmd, "--log-driver", logDriver)

	logOptsMap := d.Get("log_opts").(map[string]interface{})

	for k, v := range logOptsMap {
		cmd = append(cmd, "--log-opt", fmt.Sprintf("%s=%s", shellescape.Quote(k), shellescape.Quote(v.(string))))
	}

	envMap := d.Get("env").(map[string]interface{})

	for k, v := range envMap {
		cmd = append(cmd, "-e", fmt.Sprintf("%s=%s", shellescape.Quote(k), shellescape.Quote(v.(string))))
	}

	portsSet := d.Get("ports").(*schema.Set)
	ports := []string{}

	for _, p := range portsSet.List() {
		ports = append(ports, p.(string))
	}

	if len(ports) > 0 {
		for _, p := range ports {
			cmd = append(cmd, "-p", shellescape.Quote(p))
		}
	}

	capsSet := d.Get("caps").(*schema.Set)
	caps := []string{}

	for _, p := range capsSet.List() {
		caps = append(caps, p.(string))
	}

	if len(caps) > 0 {
		for _, c := range caps {
			cmd = append(cmd, fmt.Sprintf("--cap-add=%s", shellescape.Quote(c)))
		}
	}

	volumesSet := d.Get("volumes").(*schema.Set)
	volumes := []string{}

	for _, p := range volumesSet.List() {
		volumes = append(volumes, p.(string))
	}

	if len(volumes) > 0 {
		for _, v := range volumes {
			cmd = append(cmd, "-v", shellescape.Quote(v))
		}
	}

	memory := d.Get("memory").(int)
	if memory > 0 {
		cmd = append(cmd, "--memory", fmt.Sprintf("%d", memory))
	}

	// done with the container options, add image

	cmd = append(cmd, shellescape.Quote(imageID))

	for _, a := range d.Get("args").([]interface{}) {
		cmd = append(cmd, shellescape.Quote(a.(string)))
	}

	line := strings.Join(cmd, " ")

	output, stderr, err := sshsession.Run(d, line)
	if err != nil {
		return errors.Wrapf(err, "while running `%s`:\nSTDOUT:\n%s\nSTDERR:\n%s\n", line, string(output), string(stderr))
	}

	outputLines := strings.Split(string(output), "\n")

	if len(outputLines) < 2 {
		return errors.New("remote docker didn't return container id")
	}

	containerID := outputLines[len(outputLines)-2]

	d.Set("container_id", containerID)
	d.SetId(containerID)

	return resourceRead(d, m)
}

func resourceRead(d *schema.ResourceData, m interface{}) error {

	containerID := d.Get("container_id").(string)

	if containerID != "" {
		cmd := fmt.Sprintf("docker container inspect %s", containerID)

		output, _, err := sshsession.Run(d, cmd)
		if err != nil {
			// container does not exist
			name, nameIsSet := d.GetOkExists("name")
			if nameIsSet {
				// try inspecting by name, this can happen when `docker run` fails
				cmd = fmt.Sprintf("docker container inspect %s", name)
				output, _, err = sshsession.Run(d, cmd)
				if err != nil {
					// definitely does not exist, let the terraform know!
					d.SetId("")
					return nil
				}

			} else {
				d.SetId("")
				return nil
			}

		}

		parsed := []types.ContainerJSON{}

		err = json.Unmarshal(output, &parsed)

		if err != nil {
			return errors.Wrap(err, "while parsing docker container data")
		}

		if len(parsed) != 1 {
			return errors.Errorf("Expected 1 container info for %s, but got %d", containerID, len(parsed))
		}

		containerData := parsed[0]

		// this can change when we did lookup by container id and have failed but container with the name exists
		d.SetId(containerData.ID)
		d.Set("container_id", containerData.ID)

		_, restartSet := d.GetOk("restart")

		if restartSet {
			d.Set("restart", containerData.HostConfig.RestartPolicy.Name)
		}

		// inspect the image

		imageID := d.Get("image_id").(string)

		cmd = fmt.Sprintf("docker image inspect %s", imageID)
		output, stderr, err := sshsession.Run(d, cmd)
		if err != nil {
			errors.Wrapf(err, "while inspecting image %s:\nSTDOUT:\n%s\nSTDERR:\n%s\n", imageID, string(output), string(stderr))
		}

		parsedImages := []types.ImageInspect{}

		err = json.Unmarshal(output, &parsedImages)

		if err != nil {
			return errors.Wrapf(err, "while parsing docker images data for %s", imageID)
		}

		if len(parsed) != 1 {
			return errors.Errorf("Expected 1 container info for %s, but got %d", containerID, len(parsed))
		}

		imageInfo := parsedImages[0]
		if imageInfo.ID != imageID {
			if len(imageInfo.RepoTags) != 0 {
				d.Set("image_id", imageInfo.RepoTags[0])
			} else {
				d.Set("image_id", imageInfo.ID)
			}
		}

		// name
		_, nameIsSet := d.GetOkExists("name")
		if nameIsSet {
			d.Set("name", strings.TrimPrefix(containerData.Name, "/"))
		}

		// network
		_, networkIsSet := d.GetOkExists("network")

		if networkIsSet {
			d.Set("network", containerData.HostConfig.NetworkMode)
		}

		privileged := containerData.HostConfig.Privileged
		d.Set("privileged", privileged)

		memory := int(containerData.HostConfig.Memory)
		d.Set("memory", memory)

		// labels
		_, labelsAreSet := d.GetOkExists("labels")

		if labelsAreSet {
			l := map[string]interface{}{}

			for k, v := range containerData.Config.Labels {
				l[k] = v
			}

			for k := range imageInfo.Config.Labels {
				delete(l, k)
			}

			d.Set("labels", l)
		}

		// env
		_, envIsSet := d.GetOkExists("env")

		if envIsSet {

			env := map[string]interface{}{}

			for _, e := range containerData.Config.Env {
				se := strings.SplitN(e, "=", 2)
				if len(se) == 2 {
					env[se[0]] = se[1]
				}
			}

			for _, e := range imageInfo.Config.Env {
				se := strings.SplitN(e, "=", 2)
				if len(se) == 2 {
					v := env[se[0]]
					if v != nil && se[1] == v.(string) {
						delete(env, se[0])
					}
				}

			}

			d.Set("env", env)
		}

		// caps
		_, capsAreSet := d.GetOkExists("caps")
		if capsAreSet {
			caps := []interface{}{}
			for _, c := range containerData.HostConfig.CapAdd {
				caps = append(caps, c)
			}
			d.Set("caps", schema.NewSet(schema.HashString, caps))
		}

		// volumes
		_, volumesAreSet := d.GetOkExists("volumes")

		if volumesAreSet {
			vols := []interface{}{}
			for _, b := range containerData.HostConfig.Binds {
				vols = append(vols, b)
			}
			d.Set("volumes", schema.NewSet(schema.HashString, vols))
		}

		// ports
		_, portsAreSet := d.GetOkExists("ports")

		if portsAreSet {
			ports := []interface{}{}
			for port, bindings := range containerData.HostConfig.PortBindings {
				postfix := ""
				if port.Proto() != "tcp" {
					postfix = "/" + port.Proto()
				}

				if len(bindings) != 1 {
					continue
				}
				b := bindings[0]

				if b.HostIP != "" {
					ports = append(ports, fmt.Sprintf("%s:%s:%d%s", b.HostIP, b.HostPort, port.Int(), postfix))
					continue
				}
				ports = append(ports, fmt.Sprintf("%s:%d%s", b.HostPort, port.Int(), postfix))
			}
			d.Set("ports", schema.NewSet(schema.HashString, ports))
		}

		// args
		_, argsAreSet := d.GetOkExists("args")
		if argsAreSet {
			args := []interface{}{}
			for _, c := range containerData.Args {
				args = append(args, c)
			}
			d.Set("args", schema.NewSet(schema.HashString, args))
		}

		// log_driver
		d.Set("log_driver", containerData.HostConfig.LogConfig.Type)

		// log_opts
		logOpts := map[string]interface{}{}

		for k, v := range containerData.HostConfig.LogConfig.Config {
			logOpts[k] = v
		}

		d.Set("log_opts", logOpts)

	}

	return nil
}

func resourceUpdate(d *schema.ResourceData, m interface{}) error {

	containerID := d.Get("container_id").(string)

	if containerID != "" {
		cmd := fmt.Sprintf("docker rm -fv %s", containerID)
		output, stderr, err := sshsession.Run(d, cmd)
		if err != nil {
			return errors.Wrapf(err, "while running `%s`:\nSTDOUT:\n%s\nSTDERR:\n%s\n", cmd, string(output), string(stderr))
		}
	}

	return resourceCreate(d, m)
}

func resourceDelete(d *schema.ResourceData, m interface{}) error {

	cmd := []string{
		"docker",
		"rm",
		"-fv",
		shellescape.Quote(d.Id()),
	}

	line := strings.Join(cmd, " ")

	output, stderr, err := sshsession.Run(d, line)
	if err != nil {
		return errors.Wrapf(err, "while running `%s`:\nSTDOUT:\n%s\nSTDERR:\n%s\n", line, string(output), string(stderr))
	}

	return nil
}
