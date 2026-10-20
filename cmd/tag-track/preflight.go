package main

import "errors"

func resolveCommandFiles(args []string) ([]string, error) {
	files, err := resolveInputFiles(args)
	if err != nil {
		return nil, fileCommandFailure(files, err)
	}
	if len(files) == 0 && jsonOutput {
		return nil, fileCommandFailure(nil, errors.New("at least one input file is required"))
	}
	return files, nil
}

func fileCommandFailure(files []string, err error) error {
	if !jsonOutput {
		return err
	}
	batch := tagBatch{Status: "error", Results: []tagResult{}}
	if len(files) == 0 {
		files = []string{""}
	}
	for _, file := range files {
		batch.Results = append(batch.Results, tagResult{InputFile: file, Status: "error", Error: err.Error()})
	}
	return errors.Join(err, batch.write())
}
