package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"text/template"
	"time"
	"encoding/base64"
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

	for i := range yamlFiles {
		inpath := yamlFiles[i].Name()

		if path.Ext(inpath) != ".yaml" {
			continue
		}

		genpath := path.Join(".", GEN_FOLDER, path.Base(inpath))

		if err := templateYamlFile(params, inpath, genpath); err != nil {
			panic(err)
		}
	}

	if _, err := retryCmd(5, 3, "kubectl", "get", "nodes"); err != nil {
		panic(err)
	}

    postgresFiles := [2]string{"postgres-rc.yaml","postgres-service.yaml"}

	overlordFiles := [3]string{"dex-secrets.yaml", "dex-overlord-rc.yaml", "dex-overlord-service.yaml"}

	workerFiles := [2]string{"dex-worker-rc.yaml", "dex-worker-service.yaml"}

    //Begin pod cleanup

	for i := range workerFiles {
		f := workerFiles[i]
		_ = exec.Command("kubectl", "delete", "-f", path.Join(".", "gen", f)).Run()
	}

	for i := range overlordFiles {
		f := overlordFiles[i]
		_ = exec.Command("kubectl", "delete", "-f", path.Join(".", "gen", f)).Run()
	}

    if deployPostgres {
        for i := range postgresFiles {
            f := postgresFiles[i]
            _ = exec.Command("kubectl", "delete", "-f", path.Join(".", "gen", f)).Run()
        }
    }

    //End pod cleanup
    //Begin pod creation
    if deployPostgres {
        for i := range postgresFiles {
            f := postgresFiles[i]

            if _, err := doCmd("kubectl", "create", "-f", path.Join(".", "gen", f)); err != nil {
                panic(err)
            }
        }

        if psqlPod,err := doCmd("kubectl","get","pod" ,"-l=app=postgres","-o","template", "-t","{{ (index .items 0).metadata.name }}"); err != nil {

            panic(err)

        }else{
            if _,err := retryCmd(20,6,"kubectl","exec",string(psqlPod),"--","psql","-c","\\list",params.PostgresUrl); err != nil {
                panic(err)
            }
        }
    }

	for i := range overlordFiles {
		f := overlordFiles[i]
		if _, err := doCmd("kubectl", "create", "-f", path.Join(".", "gen", f)); err != nil {
			panic(err)
		}
	}

    for i := range(workerFiles){
        if _,err := doCmd("kubectl","create","-f",path.Join(".","gen",workerFiles[i])); err != nil {
            panic(err)
        }
    }
}

func retryCmd(maxAttempts, waitSec int, name string, args ...string) (output []byte, err error) {

	for i := 0; i < maxAttempts; i++ {
        log("(%d/%d):\n", i+1, maxAttempts)

		if output, err = doCmd(name, args...); err != nil {
			log("\terror: %s\n",err.Error())

            if i < maxAttempts - 1 {
                time.Sleep(time.Second*time.Duration(waitSec))
            }else{
                fmt.Printf("%s %v\n\t->fatal error after %d tries: %s\n",name,args,i+1,err.Error())
            }

		} else {
			log("\t->OK\n")
			return
		}
	}

	return
}

func doCmd(name string, args ...string) (output []byte, err error) {
	output, err = exec.Command(name, args...).CombinedOutput()

	log("$> %s %v\n\n", name, args)

	log("%s\n", string(output))

	return
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
