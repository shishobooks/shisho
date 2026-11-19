// The welcome page an Invite Link opens. It uses the origin the admin is
// browsing on, as Share Links do, so it works for the person invited the same
// way Shisho works for the admin.
export const inviteLinkUrl = (token: string) =>
  `${window.location.origin}/invite/${token}`;
