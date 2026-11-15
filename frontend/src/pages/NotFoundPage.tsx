import { Link } from 'react-router-dom'
import { Button, Card, EmptyState } from '../components/ui'

export function NotFoundPage() {
  return (
    <Card className="mt-8">
      <EmptyState
        title="Страница не найдена"
        description="Возможно, ссылка устарела."
        action={
          <Link to="/">
            <Button variant="primary">На главную</Button>
          </Link>
        }
      />
    </Card>
  )
}
