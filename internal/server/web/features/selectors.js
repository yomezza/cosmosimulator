import { Session } from "../core/session.js";
import { Format } from "../core/format.js";
export const d = () => Session.data.expedition,
    w = () => Session.data.workbench || {},
    res = (id) => d()?.resources.find((r) => r.id === id),
    num = (v) => Format.quantity(v),
    station = () =>
      w().stations?.find(
        (s) => s.star === d()?.ship.star && s.object_id === d()?.ship.object_id,
      ),
    fmtTime = (value) => Format.timestamp(value),
    money = (v) => Format.credits(v),
    known = (id) => w().known?.some((v) => v.star === id),
    fly = (s) => !!s.destination || !!s.local_target,
    cargoCapacity = () => d()?.ship.spec?.cargo || d()?.capacity || 60,
    fuelCapacity = () => d()?.ship.spec?.fuel || 40;
