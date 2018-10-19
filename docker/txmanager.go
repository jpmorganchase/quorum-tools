/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package docker

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"
)

type TxManager interface {
	GenerateKeys() ([]byte, []byte, error)
}

type TesseraTxManager struct {
	*DefaultConfigurable
}

func (t *TesseraTxManager) Start() error {
	return nil
}

func (t *TesseraTxManager) Stop() error {
	return nil
}

func (t *TesseraTxManager) GenerateKeys() (public []byte, private []byte, retErr error) {
	tmpDataDir, err := ioutil.TempDir("", fmt.Sprintf("qctl-%d", time.Now().Unix()))
	if err != nil {
		return nil, nil, fmt.Errorf("GenerateKeys: can't create tmp dir - %s", err)
	}
	containerWorkingDir := "/tm"
	resp, err := t.DockerClient().ContainerCreate(
		context.Background(),
		&container.Config{
			Image:      t.DockerImage(),
			WorkingDir: containerWorkingDir,
			Entrypoint: strslice.StrSlice{
				"/bin/sh",
				"-c",
				"echo \"\n\" | java -jar /tessera/tessera-app.jar -keygen",
			},
		},
		&container.HostConfig{
			Binds: []string{
				fmt.Sprintf("%s:%s", tmpDataDir, containerWorkingDir),
			},
		},
		nil,
		"",
	)
	if err != nil {
		return nil, nil, fmt.Errorf("GenerateKeys: can't create container - %s", err)
	}
	containerId := resp.ID
	if err := t.DockerClient().ContainerStart(context.Background(), containerId, types.ContainerStartOptions{}); err != nil {
		return nil, nil, fmt.Errorf("GenerateKeys: can't start container %s - %s", containerId, err)
	}
	defer t.DockerClient().ContainerRemove(context.Background(), containerId, types.ContainerRemoveOptions{Force: true})
	_, errChan := t.DockerClient().ContainerWait(context.Background(), containerId, container.WaitConditionNotRunning)
	select {
	case err := <-errChan:
		return nil, nil, fmt.Errorf("GenerateKeys: container %s is not running - %s", containerId, err)
	}

	// now read key files generated by the above run
	public, retErr = ioutil.ReadFile(filepath.Join(tmpDataDir, ".pub"))
	if retErr != nil {
		return nil, nil, retErr
	}
	private, retErr = ioutil.ReadFile(filepath.Join(tmpDataDir, ".key"))
	return
}

func NewTesseraTxManager(configureFns ...ConfigureFn) (Container, error) {
	tm := &TesseraTxManager{
		&DefaultConfigurable{},
	}
	for _, cfgFn := range configureFns {
		cfgFn(tm)
	}
	public, private, err := tm.GenerateKeys()
	if err != nil {
		return nil, err
	}
	tm.Set(CfgKeyTxManagerPublicKeys, [][]byte{public})
	tm.Set(CfgKeyTxManagerPrivateKeys, [][]byte{private})
	return tm, nil
}