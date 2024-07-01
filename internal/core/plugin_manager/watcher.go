package plugin_manager

import (
	"os"
	"path"

	"github.com/langgenius/dify-plugin-daemon/internal/core/plugin_manager/aws_manager"
	"github.com/langgenius/dify-plugin-daemon/internal/core/plugin_manager/local_manager"
	"github.com/langgenius/dify-plugin-daemon/internal/types/app"
	"github.com/langgenius/dify-plugin-daemon/internal/types/entities"
	"github.com/langgenius/dify-plugin-daemon/internal/utils/log"
	"github.com/langgenius/dify-plugin-daemon/internal/utils/parser"
)

func startWatcher(path string, platform string) {
	// load local plugins firstly
	for plugin := range loadNewPlugins(path) {
		var plugin_interface entities.PluginRuntimeInterface

		if platform == app.PLATFORM_AWS_LAMBDA {
			plugin_interface = &aws_manager.AWSPluginRuntime{
				PluginRuntime: plugin,
			}
		} else if platform == app.PLATFORM_LOCAL {
			plugin_interface = &local_manager.LocalPluginRuntime{
				PluginRuntime: plugin,
			}
		} else {
			log.Error("unsupported platform: %s for plugin: %s", platform, plugin.Config.Name)
			continue
		}

		log.Info("loaded plugin: %s:%s", plugin.Config.Name, plugin.Config.Version)
		m.Store(plugin.Config.Name, plugin_interface)

		lifetime(plugin_interface)
	}
}

// chan should be closed after using that
func loadNewPlugins(root_path string) <-chan entities.PluginRuntime {
	ch := make(chan entities.PluginRuntime)

	plugins, err := os.ReadDir(root_path)
	if err != nil {
		log.Error("no plugin found in path: %s", root_path)
		close(ch)
		return ch
	}

	go func() {
		for _, plugin := range plugins {
			if plugin.IsDir() {
				log.Info("found new plugin path: %s", plugin.Name())

				configuration_path := path.Join(root_path, plugin.Name(), "manifest.json")
				configuration, err := parsePluginConfig(configuration_path)
				if err != nil {
					log.Error("parse plugin config error: %v", err)
					continue
				}

				status := verifyPluginStatus(configuration)
				if status.exist && status.alive {
					continue
				} else if status.exist && !status.alive {
					log.Warn("plugin %s is not alive")
					continue
				}

				ch <- entities.PluginRuntime{
					Config: *configuration,
					State: entities.PluginRuntimeState{
						Restarts:     0,
						Active:       false,
						RelativePath: path.Join(root_path, plugin.Name()),
						ActiveAt:     nil,
						DeadAt:       nil,
						Verified:     false,
					},
				}
			}
		}

		close(ch)
	}()

	return ch
}

func parsePluginConfig(configuration_path string) (*entities.PluginConfiguration, error) {
	text, err := os.ReadFile(configuration_path)
	if err != nil {
		return nil, err
	}

	result, err := parser.UnmarshalJson[entities.PluginConfiguration](string(text))
	if err != nil {
		return nil, err
	}

	return &result, nil
}

type pluginStatusResult struct {
	exist bool
	alive bool
}

func verifyPluginStatus(config *entities.PluginConfiguration) pluginStatusResult {
	r, exist := checkPluginExist(config.Name)
	if exist {
		return pluginStatusResult{
			exist: true,
			alive: r.State.Active,
		}
	}

	return pluginStatusResult{
		exist: false,
		alive: false,
	}
}
