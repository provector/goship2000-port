package cockpit

import "fmt"

// Inputs are supplied by the logical notification/contact owner. Drawing does
// not decrement rows, perform duplicate filtering or dispatch audio commands.
type NotificationInputs struct {
	Rows             [4][4]byte
	ObjectNames      []string
	SecondaryName    string
	Player           byte
	LeaderA, LeaderB int8 // Raw DATA5a5/55f selectors, not a universal IFF enum.
}

func notificationColour(actor byte, in NotificationInputs) byte {
	if actor == 0 || int8(actor) > 4 {
		return 4
	}
	if int16(in.LeaderA) == int16(in.Player) {
		return 9
	}
	if int16(in.LeaderB) == int16(in.Player) {
		if int8(actor) < 3 {
			return 9
		}
		return 7
	}
	if int8(actor) >= 2 {
		return 9
	}
	return 7
}

func (in *NotificationInputs) Visible() bool {
	if in != nil {
		for _, row := range in.Rows {
			if row[0] != 0 && row[2] != 0 {
				return true
			}
		}
	}
	return false
}
func DrawNotificationsScaled(dst []byte, a *Assets, in *NotificationInputs, scale int) error {
	if in == nil {
		return nil
	}
	return LogicalPass(dst, scale, func(p []byte) error { return DrawNotifications(p, a, in) })
}

func notificationText(a *Assets, in *NotificationInputs, row [4]byte) (string, error) {
	text, ok := a.Literals[fmt.Sprintf("notification-%02d", row[0])]
	if !ok {
		return "", fmt.Errorf("prepared notification kind%d missing", row[0])
	}
	if row[1] == 0 {
		// Source1:6fb0 copies the literal HQ before the unchanged kind text.
		prefix, ok := a.Literals["notification-hq"]
		if !ok {
			return "", fmt.Errorf("prepared notification HQ prefix missing; rebuild assets")
		}
		text = prefix + text
		if row[0] == 13 {
			text += in.SecondaryName
		}
		return text, nil
	}
	name := func(raw byte) (string, error) {
		index := int(int8(raw))
		if index < 0 || index >= len(in.ObjectNames) {
			return "", fmt.Errorf("notification OBJECT name row%d unavailable", index)
		}
		return in.ObjectNames[index], nil
	}
	prefix, err := name(row[1])
	if err != nil {
		return "", err
	}
	text = prefix + text
	if row[0] == 2 {
		suffix, err := name(row[3])
		if err != nil {
			return "", err
		}
		text += suffix
	}
	return text, nil
}

// Source6e94 uses font2, white text and paired solid background4/7/9.
func DrawNotifications(dst []byte, a *Assets, in *NotificationInputs) error {
	if in == nil {
		return nil
	}
	if a == nil || !LogicalTarget(dst) {
		return fmt.Errorf("notification drawing needs source logical target/assets")
	}
	for i, row := range in.Rows {
		if row[0] == 0 || row[2] == 0 {
			continue
		}
		text, err := notificationText(a, in, row)
		if err != nil {
			return err
		}
		width, err := a.textWidth(text, 2)
		if err != nil {
			return err
		}
		half := width >> 1
		y := i * 8
		if i == 0 {
			y = 8
		}
		fill(dst, 156-half, y, 8+half*2, 7, notificationColour(row[1], *in), Full)
		if err = a.Text(dst, text, 160-half, y+1, 15, 2, Full); err != nil {
			return err
		}
		if i == 0 {
			break
		}
	}
	return nil
}
