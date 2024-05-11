package internal

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/heilkit/tt/tt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func (manager *Manager) HandlePost(p *tt.Post, threadId string) error {
	temp, err := os.MkdirTemp("", "*_tt-tg")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(temp)

	if p.IsVideo() {
		_, files, err := tt.Download(p.Id, &tt.DownloadOpt{
			Directory: temp,
			Retries:   4,
			Log:       slog.Default(),
		})
		defer func() {
			for _, file := range files {
				if err := os.Remove(file); err != nil {
					slog.Error("failed to remove temporary file", "file", file, "error", err)
				}
			}
		}()
		if err != nil {
			if _, err := manager.dscrd.ChannelMessageSend(threadId, fmt.Sprintf("#e %s", p.ID())); err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			return nil
		}

		file, err := convert(files[0], temp)
		if err != nil {
			if _, err := manager.dscrd.ChannelMessageSend(threadId, fmt.Sprintf("#e %s", p.ID())); err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			return nil
		}
		defer file.Close()
		if _, err := manager.dscrd.ChannelFileSend(threadId, path.Base(files[0]), file); err != nil {
			return fmt.Errorf("failed to send video: %w", err)
		}

		return nil
	}

	if p.HdSize > 25*(1<<20) {
		slog.Warn("post size is too big", "size mb", float64(p.HdSize)/(25*(1<<20)))
		if _, err := manager.dscrd.ChannelMessageSend(threadId, fmt.Sprintf("#big %s", p.ID())); err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
		return nil
	}

	_, files, err := tt.Download(p.Id, &tt.DownloadOpt{
		Directory: temp,
		FilenameFormat: func(post *tt.Post, i int) string {
			return fmt.Sprintf("@%s_%s_%d.jpg", post.Author.UniqueId, time.Unix(post.CreateTime, 0).Format(time.DateOnly), i)
		},
		Retries: 4,
		Log:     slog.Default(),
	})
	if err != nil {
		if _, err := manager.dscrd.ChannelMessageSend(threadId, fmt.Sprintf("#e %s", p.ID())); err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
		return nil
	}
	for _, file := range files {
		file, err := os.Open(file)
		if err != nil {
			if _, err := manager.dscrd.ChannelMessageSend(threadId, fmt.Sprintf("#e %s", p.ID())); err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			return nil
		}
		defer file.Close()
		if _, err := manager.dscrd.ChannelFileSend(threadId, path.Base(files[0]), file); err != nil {
			return fmt.Errorf("failed to send document: %w", err)
		}
	}

	return nil
}

func (manager *Manager) Profile(profile *Profile) error {
	if profile.UserId == "" {
		slog.Info("no user id, getting it", "profile", profile.Tag)
		info, err := tt.GetUserDetail(profile.Username)
		if err != nil {
			return fmt.Errorf("failed to get user info: %w", err)
		}
		profile.UserId = info.User.Id
		if err := manager.Config.Update(); err != nil {
			return fmt.Errorf("failed to update config: %w", err)
		}
	}

	if profile.Thread == "" {
		msg, err := manager.dscrd.ChannelMessageSend(manager.Config.Chat, profile.Tag)
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}

		slog.Info("no thread id, creating a thread", "profile", profile.Tag)
		topic, err := manager.dscrd.MessageThreadStartComplex(manager.Config.Chat, msg.ID, &discordgo.ThreadStart{
			Name:                profile.Tag,
			AutoArchiveDuration: 0,
			Invitable:           true,
			RateLimitPerUser:    0,
		})
		if err != nil {
			return fmt.Errorf("failed to create topic: %w", err)
		}
		profile.Thread = topic.ID
		if err := manager.Config.Update(); err != nil {
			return fmt.Errorf("failed to update config: %w", err)
		}
	}

	postChan, expectedCount, err := tt.GetUserFeed(profile.Username, tt.FeedOpt{
		While: tt.WhileAfter(profile.LastUpload),
		OnError: func(err error) {
			if err != nil {
				slog.Error("failed to get user feed", "err", err)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get user feed: %w", err)
	}
	if expectedCount == 0 {
		slog.Info("No updates", "user", profile.Tag)
		return nil
	}

	i := 0
	var post tt.Post
	for post = range postChan {
		i += 1
		slog.Info(fmt.Sprintf("Getting post [%d/%d]", i, expectedCount), "user", profile.Tag)
		if err := manager.HandlePost(&post, profile.Thread); err != nil {
			return fmt.Errorf("failed to handle post: %w (%s)", err, post.Id)
		}
	}
	profile.LastUpload = time.Unix(post.CreateTime, 0)
	if err := manager.Config.Update(); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	return nil
}

func convert(filename, dir string) (converted *os.File, err error) {
	scaleRule := makeScaleRule(1920, 1920)

	tmpFile, err := os.CreateTemp(dir, fmt.Sprintf("*_heilkit_tg_%s", filetype(filename)))
	if err != nil {
		return nil, err
	}

	output, err := exec.Command("ffmpeg", "-y",
		"-i", filename,
		"-vf", scaleRule,
		"-vcodec", "libx264",
		"-acodec", "aac",
		"-preset", "fast",
		tmpFile.Name()).
		CombinedOutput()
	if err != nil {
		_ = tmpFile.Close()
		return nil, wrapExecError(err, output)
	}

	return os.Open(tmpFile.Name())
}

// makeScaleRule: https://stackoverflow.com/questions/54063902/resize-videos-with-ffmpeg-keep-aspect-ratio
func makeScaleRule(width int, height int) string {
	return fmt.Sprintf("scale=if(gte(iw\\,ih)\\,min(%d\\,iw)\\,-2):if(lt(iw\\,ih)\\,min(%d\\,ih)\\,-2)", width, height)
}

func filetype(filename string) string {
	index := strings.LastIndexByte(filename, '.')
	if index < 0 {
		return filepath.Base(filename)
	}
	return filename[index:]
}

func wrapExecError(err error, output []byte) error {
	if err == nil || len(output) == 0 {
		return err
	}
	return fmt.Errorf("err: %s\nout: %s", err.Error(), string(output))
}
