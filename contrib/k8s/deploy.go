package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"text/template"
	"encoding/base64"
	"regexp"
)

type DexKubeParams struct {
	KeySecrets  string
	PostgresUrl string
	K8sHost     string
    ConnectorsJSON string
	WorkerCount int
}

const (
	GEN_FOLDER = "./gen"
    DEFAULT_POSTGRES_URL = "postgres://postgres@dex-postgres.default:5432/postgres?sslmode=disable"
)


func log(pattern string,args ...interface{}){
    if verbose {
        fmt.Printf(pattern,args...)
    }
}
var verbose bool

func main() {
	params := DexKubeParams{}

    var deployPostgres bool

    flag.BoolVar(&verbose,"verbose",false,"verbose output")

    flag.BoolVar(&deployPostgres,"deploy-postgres",false,"deploy postgres pod and service at <postgres-url>")

	flag.StringVar(&params.KeySecrets, "key-secrets", "ZUhoNGVIaDRlSGg0ZUhoNGVIaDRlSGg0ZUhoNGVIaDRlSGg0ZUhoNGVIZz0=", "base64 key used to encrypt secrets")

	flag.StringVar(&params.PostgresUrl, "postgres-url", DEFAULT_POSTGRES_URL , "postgres database url")

	flag.StringVar(&params.K8sHost, "k8s-host", "http://172.17.4.99:30556", "k8s controller host")

    var connectorPath string
    flag.StringVar(&connectorPath,"connectors-json-file","./connectors.json","JSON file containing array of connector objects")

	flag.IntVar(&params.WorkerCount, "worker-count", 1, "how dex many workers")

	flag.Parse()

    if cJsonBytes, err := ioutil.ReadFile(connectorPath); err != nil {
        panic(err)
    }else{
        params.ConnectorsJSON = base64.StdEncoding.EncodeToString(cJsonBytes)
    }

    if deployPostgres && params.PostgresUrl != DEFAULT_POSTGRES_URL {
        panic(fmt.Errorf("Defining postgres-url and deploy-postgres is not supported"))
    }

	if err := os.RemoveAll(GEN_FOLDER); err != nil {
		panic(err)
	}

	if err := os.Mkdir(GEN_FOLDER, os.ModeDir|0700); err != nil {
		panic(err)
	}

	yamlFiles, err := ioutil.ReadDir("./")

	if err != nil {
		panic(err)
	}

    postgresRegexp := regexp.MustCompile("^postgres-.*.yaml$")

	for i := range yamlFiles {
		inpath := yamlFiles[i].Name()

		if path.Ext(inpath) != ".yaml" {
			continue
		}

        if !deployPostgres && postgresRegexp.MatchString(path.Base(inpath)) {
            // if we're NOT deploying postgres, and this is a postgres-*.yaml file -->
            // then skip it
            continue
        }

		genpath := path.Join(".", GEN_FOLDER, path.Base(inpath))

		if err := templateYamlFile(params, inpath, genpath); err != nil {
			panic(err)
		}
	}

    genPath := path.Join(".","gen")

    if output,err := exec.Command("kubectl","delete","-f",genPath).CombinedOutput(); err != nil {
        fmt.Printf("output: %s\n",output)
        panic(err)
    }

    if output,err := exec.Command("kubectl","create","-f",genPath).CombinedOutput(); err != nil {
        fmt.Printf("output: %s\n",output)
        panic(err)
    }
}

func templateYamlFile(params DexKubeParams, inpath, genpath string) error {
	if tmpl, err := template.New(path.Base(inpath)).ParseFiles(inpath); err != nil {

		return err

	} else {

		f, err := os.OpenFile(genpath, os.O_WRONLY|os.O_CREATE, 0700)

		if err != nil {
			return err
		}

		defer f.Close()

		return tmpl.Execute(f, params)
	}
}
