package slurmer

import (
	"bufio"
	"encoding/json"
	"github.com/ShinoYasx/Slurmer/pkg/slurmer"
	"io"
	"log"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

func Response(w http.ResponseWriter, v interface{}, code int) {
	jsonData, err := json.Marshal(v)
	if err != nil {
		Error(w, http.StatusInternalServerError)
		panic(err)
	}

	w.WriteHeader(code)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(jsonData)
	if err != nil {
		panic(err)
	}
}

func Error(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

func Ok(w http.ResponseWriter, v interface{}) {
	Response(w, v, http.StatusOK)
}

func WriteBatch(out io.Writer, batch *slurmer.BatchProperties) error {
	tmpl, err := template.ParseFiles(filepath.Join("templates", "batch.tmpl"))
	if err != nil {
		return err
	}
	return tmpl.Execute(out, batch)
}

func handleStartJob(job *slurmer.Job) error {
	cmd := exec.Command("sbatch", "--wait", "batch.sh")
	cmd.Dir = job.Directory
	jobStdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	go func() {
		// Goroutine will get slurm job id and wait for the job to end, so it can change its status
		scanner := bufio.NewScanner(jobStdout)
		// Read the first line of sbatch to get the slurm job id
		if scanner.Scan() {
			submitLine := scanner.Text()
			words := strings.Split(submitLine, " ")
			job.CurrentSlurmID, err = strconv.Atoi(words[len(words)-1])
			if err != nil {
				log.Panic(err)
			}
		}
		err = cmd.Wait()
		if err != nil {
			log.Panic(err)
		}
		// When the job is terminated, mark the job as stopped
		job.Status = slurmer.JobStatus.Stopped
		job.CurrentSlurmID = 0 // 0 is job not active
	}()

	job.Status = slurmer.JobStatus.Started
	return nil
}

func handleStopJob(job *slurmer.Job) error {
	return exec.Command("scancel", strconv.Itoa(job.CurrentSlurmID)).Start()
}