package scheduler

import (
	"encoding/json"
	"github.com/hoisie/web"
	log "github.com/ngaut/logging"
	"io/ioutil"
	_ "net/http/pprof"
	"strings"
	"time"

	"github.com/hoisie/web"
)

var pages map[string]string

func init() {
	pages = make(map[string]string)
	job, _ := ioutil.ReadFile("./templates/job.html")
	dag, _ := ioutil.ReadFile("./templates/dag.html")
	status, _ := ioutil.ReadFile("./templates/status.html")
	pages["index"] = string(job)
	pages["job"] = string(job)
	pages["dag"] = string(dag)
	pages["status"] = string(status)
}

type Notifier interface {
	OnRunJob(name string) (string, error) //taskId, error
	GetStatusByTaskId(taskId string) (string, error)
}

type Server struct {
	addr     string
	notifier Notifier
}

var (
	s *Server
)

func NewServer(addr string, notifier Notifier) *Server {
	if s != nil {
		return s
	}
	s = &Server{addr, notifier}
	return s
}

func responseJson(ctx *web.Context, statusCode int, obj interface{}) string {
	ctx.WriteHeader(statusCode)
	if obj != nil {
		content, _ := json.MarshalIndent(obj, " ", "  ")
		return string(content)
	}
	return ""
}

func responseError(ctx *web.Context, ret int, msg string) string {
	return responseJson(ctx, 200, map[string]interface{}{
		"ret": ret,
		"msg": msg,
	})
}

func responseSuccess(ctx *web.Context, data interface{}) string {
	return responseJson(ctx, 200, map[string]interface{}{
		"ret":  0,
		"data": data,
	})
}

func jobList(ctx *web.Context) string {
	jobs := GetJobList()
	if jobs != nil && len(jobs) > 0 {
		return responseSuccess(ctx, jobs)
	}
	return responseSuccess(ctx, nil)
}

func jobUpdate(ctx *web.Context) string {
	name, b := ctx.Params["name"]
	if !b {
		return responseError(ctx, -1, "job name is needed")
	}

	if JobExists(name) {
		b, err := ioutil.ReadAll(ctx.Request.Body)
		if err != nil {
			return responseError(ctx, -2, err.Error())
		}
		var job Job
		err = json.Unmarshal(b, &job)
		j, _ := GetJobByName(name)
		if err != nil {
			return responseError(ctx, -3, err.Error())
		}
		job.Id = j.Id
		if err := job.Save(); err != nil {
			return responseError(ctx, -4, err.Error())
		}
		return responseSuccess(ctx, job)
	} else {
		return responseError(ctx, -5, "no such job")
	}
}

func jobNew(ctx *web.Context) string {
	b, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		return responseError(ctx, -1, err.Error())
	}
	var job Job
	err = json.Unmarshal(b, &job)
	job.CreateTs = time.Now().Unix()
	if err != nil {
		return responseError(ctx, -2, err.Error())
	}
	err = sharedDbMap.Insert(&job)
	if err != nil {
		return responseError(ctx, -3, err.Error())
	}
	return responseSuccess(ctx, job)
}

func jobRemove(ctx *web.Context) string {
	name, b := ctx.Params["name"]
	if !b {
		return responseError(ctx, -1, "job name is needed")
	}
	j, _ := GetJobByName(name)
	if j != nil {
		if err := j.Remove(); err != nil {
			return responseError(ctx, -2, err.Error())
		}
		return responseSuccess(ctx, j)
	}
	return responseError(ctx, -3, "no such job")
}

func dagList(ctx *web.Context) string {
	dagMetaList := GetDagMetaList()
	return responseSuccess(ctx, dagMetaList)
}

func dagNew(ctx *web.Context) string {
	b, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		return responseError(ctx, -1, err.Error())
	}
	var dag DagMeta
	err = json.Unmarshal(b, &dag)
	dag.CreateTs = time.Now().Unix()

	var params map[string]interface{}
	err = json.Unmarshal(b, &params)

	if err != nil {
		return responseError(ctx, -2, err.Error())
	}

	if b, exists := params["jobs"]; exists {
		jobNames := strings.Split(b.(string), "\n")
		for _, line := range jobNames {
			pair := strings.Split(line, ",")
			jobName := strings.TrimSpace(pair[0])
			parentName := ""
			if len(pair) == 2 {
				parentName = strings.TrimSpace(pair[1])
			}
			if len(jobName) > 0 {
				j := NewDagJob(jobName, parentName)
				log.Info(j)
				dag.AddDagJob(j)
			}
		}
	}

	err = dag.Save()
	if err != nil {
		return responseError(ctx, -3, err.Error())
	}
	return responseSuccess(ctx, dag)
}

func dagRemove(ctx *web.Context) string {
	name, b := ctx.Params["name"]
	if !b {
		return responseError(ctx, -1, "dag name is needed")
	}
	dag := GetDagFromName(name)
	if dag != nil {
		if err := dag.Remove(); err != nil {
			return responseError(ctx, -2, err.Error())
		}
		return responseSuccess(ctx, dag)
	}
	return responseError(ctx, -3, "no such dag")
}

func dagJobAdd(ctx *web.Context) string {
	name, ok := ctx.Params["name"]
	if !ok {
		return responseError(ctx, -1, "dag job name is needed")
	}
	b, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		return responseError(ctx, -2, err.Error())
	}
	dag := GetDagFromName(name)
	if dag != nil {
		var job DagJob
		err = json.Unmarshal(b, &job)
		if err != nil {
			return responseError(ctx, -3, err.Error())
		}
		err = dag.AddDagJob(&job)
		if err != nil {
			return responseError(ctx, -4, err.Error())
		}
		return responseSuccess(ctx, job)
	}
	return responseError(ctx, -5, "same job exists")
}

func dagJobRemove(ctx *web.Context) string {
	name, ok := ctx.Params["name"]
	if !ok {
		return responseError(ctx, -1, "dag job name is needed")
	}
	dag := GetDagFromName(name)
	if dag != nil {
		err := dag.Remove()
		if err != nil {
			return responseError(ctx, -2, err.Error())
		}
		return responseSuccess(ctx, "")
	} else {
		return responseError(ctx, -3, "no such dag job")
	}
}

func dagJobRun(ctx *web.Context) string {
	name, ok := ctx.Params["name"]
	if !ok {
		return responseError(ctx, -1, "dag job name is needed")
	}

	if s.notifier != nil {
		taskId, err := s.notifier.OnRunJob(name)
		if err != nil {
			return responseError(ctx, -2, err.Error())
		}
		return responseSuccess(ctx, taskId)
	}
	return responseError(ctx, -3, "notifier not found")
}

func jobPage(ctx *web.Context) string {
	return pages["job"]
}

func dagPage(ctx *web.Context) string {
	return pages["dag"]
}

func statusPage(ctx *web.Context) string {
	return pages["status"]
}

func indexPage(ctx *web.Context) string {
	return pages["index"]
}

func (srv *Server) Serve() {
	web.Get("/", indexPage)
	web.Get("/job", jobPage)
	web.Get("/dag", dagPage)
	web.Get("/status", statusPage)

	web.Get("/job/list", jobList)
	web.Get("/dag/list", dagList)

	web.Post("/job/new", jobNew)
	web.Post("/job/remove", jobRemove)
	web.Post("/job/update", jobUpdate)
	web.Post("/dag/new", dagNew)
	web.Post("/dag/remove", dagRemove)
	web.Post("/dag/job/add", dagJobAdd)
	web.Post("/dag/job/remove", dagJobRemove)
	web.Post("/dag/job/run", dagJobRun)
	addr, _ := globalCfg.ReadString("http_addr", ":9090")
	web.Run(addr)
}
