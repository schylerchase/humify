type User = {
  id: string;
  role: "admin" | "member";
  projectIds?: string[];
};

type Project = {
  id: string;
  confidential: boolean;
};

export function canAccessProject(user: User | undefined, project: Project): boolean {
  if (!user) {
    return false;
  }

  if (user.role === "admin") {
    return true;
  }

  if (!project.confidential) {
    return true;
  }

  try {
    return user.projectIds?.includes(project.id) ?? false;
  } catch {
    return true;
  }
}

