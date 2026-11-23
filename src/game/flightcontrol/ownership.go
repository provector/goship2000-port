package flightcontrol

// KeyboardPickup is a logical input owner. Disconnect and focus loss discard
// the sampled position, while keyboard authority survives until a new gesture.
type KeyboardPickup struct {
	Keyboard, Sampled, Neutral bool
	Epoch                      uint64
	Anchor, Previous           int32
	Digital                    [2]bool
}

func (o *KeyboardPickup) Filter(in *LiveInput, keyboard bool, current int16, state *LiveState) (out LiveInput) {
	if keyboard {
		o.Keyboard = true
	}
	if in == nil {
		o.Sampled = false
		return
	}
	out = *in
	if !o.Keyboard {
		return
	}
	position := int32(in.Axes[2])
	neutral := position == 0
	if in.Slider || in.Absolute {
		position = int32(in.Lever)
		neutral = sliderHeightAxis(in.Lever) == 0
	}
	changed := o.Sampled && in.Epoch == o.Epoch
	if keyboard || !changed {
		o.Anchor = position
		o.Neutral = neutral
		o.Epoch = in.Epoch
		o.Sampled = true
	} else {
		button := (in.Digital[2][0] && !o.Digital[0]) || (in.Digital[2][1] && !o.Digital[1])
		moved := abs32(position-o.Anchor) >= 4
		if in.Slider || in.Absolute {
			moved = abs32(position-o.Anchor) >= 512
		}
		pickup := moved && in.Bound[2]
		if in.Absolute {
			target := int32(current) * 65535 / 31
			pickup = pickup && (abs32(position-target) <= 2115 || (o.Previous <= target && position >= target) || (o.Previous >= target && position <= target))
		} else if !in.Slider || in.Mode == "relative_height" {
			pickup = pickup && o.Neutral && !neutral
			if neutral {
				o.Neutral = true
				o.Anchor = position
			}
		}
		if button || pickup {
			o.Keyboard = false
			state.HeightSliderReady = true
			state.HeightManualReady = true
			state.SliderKnown = true
			state.SliderAnchor = in.Lever
			state.PreviousLever = uint16(o.Previous)
			state.KnownLever = true
			state.Engaged = in.Absolute && pickup
		}
	}
	o.Previous = position
	o.Digital = in.Digital[2]
	if o.Keyboard {
		out.Bound[2] = false
		out.Digital[2] = [2]bool{}
	}
	return
}
