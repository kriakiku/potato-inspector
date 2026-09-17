import { toast } from 'sonner'

export function notifySuccess(message) {
  toast.success(message)
}

export function notifyError(message) {
  toast.error(message || 'Something went wrong')
}

export function notifyInfo(message) {
  toast.message(message)
}
