package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/openfaas/faas-provider/auth"
	"github.com/openfaas/faas-provider/types"
	"github.com/openfaas/go-sdk"
)

func main() {

	var (
		gatewaySource string
		gatewayTarget string

		dryRun bool
	)

	flag.StringVar(&gatewaySource, "source", "http://admin@$PASSWORD:127.0.0.1:8080", "Originating gateway address")
	flag.StringVar(&gatewayTarget, "target", "http://admin@$PASSWORD:127.0.0.1:8081", "Target gateway address")

	flag.BoolVar(&dryRun, "dry-run", false, "Dry run")

	flag.Parse()

	uSource, err := url.Parse(gatewaySource)
	if err != nil {
		log.Fatal(err)
	}

	uTarget, err := url.Parse(gatewayTarget)
	if err != nil {
		log.Fatal(err)
	}

	sourcePass, _ := uSource.User.Password()
	sourceCreds := auth.BasicAuthCredentials{
		User:     uSource.User.Username(),
		Password: sourcePass,
	}

	targetPass, _ := uTarget.User.Password()
	targetCreds := auth.BasicAuthCredentials{
		User:     uTarget.User.Username(),
		Password: targetPass,
	}

	sourceSdk := sdk.NewClient(uSource, &sourceCreds, http.DefaultClient)
	targetSdk := sdk.NewClient(uTarget, &targetCreds, http.DefaultClient)

	if info, err := sourceSdk.GetInfo(); err != nil {
		log.Fatal(err)
	} else {
		fmt.Printf(`Source:
 - %s/%s
 - version: %s
 - commit: %s
 
 `,
			info.Provider.Orchestration, info.Provider.Provider, info.Provider.Version.Release, info.Provider.Version.SHA)
	}
	if info, err := targetSdk.GetInfo(); err != nil {
		log.Fatal(err)
	} else {
		fmt.Printf(`Target:
 - %s/%s
 - version: %s
 - commit: %s
 
 `,
			info.Provider.Orchestration, info.Provider.Provider, info.Provider.Version.Release, info.Provider.Version.SHA)
		if strings.Contains(info.Provider.Provider, "ce") {
			log.Fatal("Invalid target cluster")
		}
	}

	fmt.Println()
	if err := mirror(uSource, sourceSdk, uTarget, targetSdk, dryRun); err != nil {
		log.Fatal(err)
	}
}

func mirror(uSource *url.URL, sourceSdk *sdk.Client, uTarget *url.URL, targetSdk *sdk.Client, dryRun bool) error {
	sourceFns, err := sourceSdk.GetFunctions("openfaas-fn")
	if err != nil {
		return err
	}

	deployed := map[string]types.FunctionDeployment{}

	for _, summary := range sourceFns {
		spec, err := sourceSdk.GetFunction(summary.Name, "openfaas-fn")
		if err != nil {
			return err
		}
		deployed[summary.Name] = spec
	}

	for name, spec := range deployed {
		fmt.Printf("=> Deploy: %s to target cluster\n", name)
		if dryRun {
			continue
		}

		existing := false
		if _, err := targetSdk.GetFunction(name, spec.Namespace); err == nil {
			existing = true
		}

		fn := types.FunctionDeployment{
			Service:                name,
			Image:                  spec.Image,
			EnvProcess:             spec.EnvProcess,
			EnvVars:                spec.EnvVars,
			Labels:                 spec.Labels,
			Annotations:            spec.Annotations,
			Constraints:            spec.Constraints,
			Limits:                 spec.Limits,
			Requests:               spec.Requests,
			ReadOnlyRootFilesystem: spec.ReadOnlyRootFilesystem,
			Namespace:              spec.Namespace,
			Secrets:                spec.Secrets,
		}

		var deployStatusCode int
		if !existing {
			deployStatusCode, err = targetSdk.Deploy(fn)
			if err != nil {
				return err
			}
		} else {
			deployStatusCode, err = targetSdk.Update(fn)
			if err != nil {
				return err
			}
		}

		method := "POST"
		if existing {
			method = "PUT"
		}

		fmt.Printf("<= %s: %d [%s]\n", name, deployStatusCode, method)
	}

	return nil
}
