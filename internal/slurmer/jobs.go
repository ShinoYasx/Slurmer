package slurmer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ShinoYasx/Slurmer/pkg/slurm"
	"github.com/ShinoYasx/Slurmer/pkg/slurmer"
	"github.com/go-chi/chi"
	"github.com/google/uuid"
)

func (srv *Server) jobsRouter(r chi.Router) {
	r.Get("/", srv.listJobs)
	r.Post("/", srv.createJob)
	r.Route("/{jobID}", func(r chi.Router) {
		r.Use(srv.JobCtx)
		r.Get("/", srv.getJob)
		r.Put("/status", srv.updateJobStatus)
		r.Route("/files", filesRouter)
	})
}

func (srv *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(*slurmer.Application)

	Response(w, app.Jobs)
}

func (srv *Server) getJob(w http.ResponseWriter, r *http.Request) {
	job := r.Context().Value("job").(*slurmer.Job)

	// if job.Status == slurmer.JobStatus.Started {
	// 	jobProp, err := srv.slurmClient.GetJob(job.CurrentSlurmID)
	// 	if err != nil {
	// 		Error(w, http.StatusInternalServerError)
	// 		panic(err)
	// 	}
	// 	job.SlurmJob = jobProp
	// }

	Response(w, job)
	// job.SlurmJob = nil
}

func (srv *Server) createJob(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value("app").(*slurmer.Application)

	var jobID string
	// Debug purposes
	if app.ID == "debug" {
		jobID = "debug"
	} else {
		jobID = uuid.New().String()
	}

	reqBody, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		Error(w, http.StatusInternalServerError)
		panic(err)
	}

	jobDir := filepath.Join(app.Directory, "jobs", jobID)

	err = os.MkdirAll(jobDir, os.ModePerm)
	if err != nil {
		Error(w, http.StatusInternalServerError)
		panic(err)
	}

	batchFile, err := os.Create(filepath.Join(jobDir, "batch.sh"))
	if err != nil {
		Error(w, http.StatusInternalServerError)
		panic(err)
	}
	defer batchFile.Close()

	var batchProperties slurm.BatchProperties
	err = json.Unmarshal(reqBody, &batchProperties)
	if err != nil {
		Error(w, http.StatusInternalServerError)
		panic(err)
	}

	err = WriteBatch(batchFile, &batchProperties)
	if err != nil {
		Error(w, http.StatusInternalServerError)
		panic(err)
	}

	job := slurmer.Job{
		Name:      batchProperties.JobName,
		Status:    slurmer.JobStatus.Stopped,
		ID:        jobID,
		Directory: jobDir,
	}

	app.Jobs.AddJob(jobID, &job)

	w.WriteHeader(http.StatusCreated)
	Response(w, &job)
}

func (srv *Server) updateJobStatus(w http.ResponseWriter, r *http.Request) {
	job := r.Context().Value("job").(*slurmer.Job)

	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		Error(w, http.StatusBadRequest)
		panic(err)
	}
	defer r.Body.Close()

	var status string
	if err = json.Unmarshal(reqBody, &status); err != nil {
		Error(w, http.StatusBadRequest)
		panic(err)
	}

	switch status {
	case "started":
		if job.Status == slurmer.JobStatus.Stopped {
			if err := srv.handleStartJob(job); err != nil {
				Error(w, http.StatusInternalServerError)
				panic(err)
			}
		}
	case "stopped":
		if job.Status == slurmer.JobStatus.Started {
			if err := srv.slurmClient.CancelJob(job.CurrentSlurmID); err != nil {
				Error(w, http.StatusInternalServerError)
				panic(err)
			}
		}
	}

	Response(w, status)
}

func (srv *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := ctx.Value("app").(*slurmer.Application)
	job := ctx.Value("job").(*slurmer.Job)

	// First we need to stop pending/running job
	if job.Status == slurmer.JobStatus.Started {
		err := srv.slurmClient.CancelJob(job.CurrentSlurmID)
		if err != nil {
			Error(w, http.StatusInternalServerError)
			panic(err)
		}
	}

	app.Jobs.DeleteJob(job.ID)

	Error(w, http.StatusOK)
}

func (srv *Server) JobCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app := r.Context().Value("app").(*slurmer.Application)
		jobID := chi.URLParam(r, "jobID")
		job, err := app.Jobs.GetJob(jobID)
		if err != nil {
			Error(w, http.StatusNotFound)
			return
		}

		ctx := context.WithValue(r.Context(), "job", job)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
