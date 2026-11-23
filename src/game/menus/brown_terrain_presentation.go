package menus

import "gunship2000-assets/game/render3d"

func groundOwnerAdmits(owner, index byte) bool {
	return render3d.SurfaceOwner(owner).ApprovedGround()
}
